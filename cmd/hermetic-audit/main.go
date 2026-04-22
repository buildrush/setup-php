// Package main implements hermetic-audit, which verifies a built bundle's ELF
// files resolve all their shared-library dependencies when loaded on a given
// runner OS. Uses ldd inside a Docker image matching --expected-runner-os so
// the audit reflects the apt reality of the target runner, not the CI host.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type metaSidecar struct {
	HermeticLibs []string `json:"hermetic_libs"`
	Kind         string   `json:"kind"`
}

func main() {
	bundlePath := flag.String("bundle", "", "path to extracted bundle directory")
	runnerOS := flag.String("expected-runner-os", "", "e.g. ubuntu-22.04 or ubuntu-24.04")
	format := flag.String("format", "human", "human|json")
	flag.Parse()

	if *bundlePath == "" || *runnerOS == "" {
		fmt.Fprintln(os.Stderr, "usage: hermetic-audit --bundle <dir> --expected-runner-os <os>")
		os.Exit(2)
	}

	meta, err := readMeta(filepath.Join(*bundlePath, "meta.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "::error::hermetic-audit: %v\n", err)
		os.Exit(2)
	}

	elves, err := findELFs(*bundlePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "::error::hermetic-audit: find ELFs: %v\n", err)
		os.Exit(2)
	}

	var report auditReport
	report.Bundle = *bundlePath
	report.RunnerOS = *runnerOS
	for _, elf := range elves {
		missing, err := lddInDocker(elf, *runnerOS)
		if err != nil {
			fmt.Fprintf(os.Stderr, "::error::hermetic-audit: ldd %s: %v\n", elf, err)
			os.Exit(2)
		}
		unexplained, captureBugs := classifyMissing(missing, meta.HermeticLibs)
		for _, u := range unexplained {
			report.Unexplained = append(report.Unexplained, findingEntry{ELF: elf, Name: u, Suggest: suggestGlob(u)})
		}
		for _, c := range captureBugs {
			report.CaptureBugs = append(report.CaptureBugs, findingEntry{ELF: elf, Name: c})
		}
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	default:
		printHuman(&report)
	}

	if len(report.Unexplained) > 0 || len(report.CaptureBugs) > 0 {
		os.Exit(1)
	}
}

type findingEntry struct {
	ELF     string `json:"elf"`
	Name    string `json:"name"`
	Suggest string `json:"suggest_glob,omitempty"`
}

type auditReport struct {
	Bundle      string         `json:"bundle"`
	RunnerOS    string         `json:"runner_os"`
	Unexplained []findingEntry `json:"unexplained"`
	CaptureBugs []findingEntry `json:"capture_bugs"`
}

func readMeta(path string) (*metaSidecar, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m metaSidecar
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}

// findELFs returns absolute paths to ELF files in the bundle. It uses the ELF
// magic (0x7F 'E' 'L' 'F') to detect binaries across architectures.
func findELFs(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil //nolint:nilerr // best-effort: skip unreadable files
		}
		magic := make([]byte, 4)
		_, readErr := f.Read(magic)
		_ = f.Close()
		if readErr != nil {
			return nil //nolint:nilerr // best-effort: skip files shorter than 4 bytes
		}
		if bytes.Equal(magic, []byte{0x7F, 'E', 'L', 'F'}) {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// lddInDocker runs `ldd` on the ELF inside a Docker container matching the
// given runner OS. Returns the list of "not found" library names.
func lddInDocker(elf, runnerOS string) ([]string, error) {
	image := map[string]string{
		"ubuntu-22.04":     "ubuntu:22.04",
		"ubuntu-24.04":     "ubuntu:24.04",
		"ubuntu-22.04-arm": "ubuntu:22.04",
		"ubuntu-24.04-arm": "ubuntu:24.04",
	}[runnerOS]
	if image == "" {
		return nil, fmt.Errorf("unsupported --expected-runner-os %q", runnerOS)
	}
	dir := filepath.Dir(elf)
	base := filepath.Base(elf)
	// Guard: base must not contain path separators or spaces so it is safe
	// to embed in the mount path without further shell quoting.
	if strings.ContainsAny(base, "/ ") {
		return nil, fmt.Errorf("unexpected characters in ELF filename %q", base)
	}
	mountArg := dir + ":/audit:ro"
	elfArg := "/audit/" + base
	cmd := exec.Command("docker", "run", "--rm", //nolint:gosec // base validated above; image from a fixed allow-list
		"-v", mountArg,
		"--entrypoint", "ldd",
		image,
		elfArg)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker ldd: %w", err)
	}
	return parseLddNotFound(stdout.String()), nil
}

func parseLddNotFound(output string) []string {
	var missing []string
	sc := bufio.NewScanner(strings.NewReader(output))
	re := regexp.MustCompile(`^\s*(\S+)\s*=>\s*not found`)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			missing = append(missing, m[1])
		}
	}
	return missing
}

// classifyMissing splits a list of missing libs into (unexplained, captureBugs)
// by matching each name against the declared hermetic_libs globs.
func classifyMissing(missing, declared []string) (unexplained, captureBugs []string) {
	for _, name := range missing {
		matched := false
		for _, glob := range declared {
			if matchGlob(glob, name) {
				matched = true
				break
			}
		}
		if matched {
			captureBugs = append(captureBugs, name)
		} else {
			unexplained = append(unexplained, name)
		}
	}
	return unexplained, captureBugs
}

// matchGlob applies Go filepath.Match with the glob's simple shell-style
// semantics. Hermetic globs never contain '/' (validated in the catalog),
// which keeps filepath.Match behavior intuitive.
func matchGlob(glob, name string) bool {
	ok, _ := filepath.Match(glob, name)
	return ok
}

// suggestGlob turns a concrete missing lib name into a forward-compat glob
// suggestion.
func suggestGlob(name string) string {
	if idx := strings.Index(name, ".so."); idx >= 0 {
		return name[:idx+4] + "*"
	}
	if strings.HasSuffix(name, ".so") {
		return name + ".*"
	}
	return name
}

func printHuman(r *auditReport) {
	if len(r.Unexplained) == 0 && len(r.CaptureBugs) == 0 {
		fmt.Printf("hermetic-audit: %s OK on %s\n", r.Bundle, r.RunnerOS)
		return
	}
	if len(r.Unexplained) > 0 {
		fmt.Printf("::error::hermetic-audit: %d lib(s) not resolvable on %s and not declared in hermetic_libs:\n", len(r.Unexplained), r.RunnerOS)
		for _, f := range r.Unexplained {
			fmt.Printf("  %s missing %s  →  add %q to hermetic_libs of %s\n",
				f.ELF, f.Name, f.Suggest, r.Bundle)
		}
	}
	if len(r.CaptureBugs) > 0 {
		fmt.Printf("::error::hermetic-audit: %d lib(s) declared in hermetic_libs but still missing on %s (capture-script bug):\n", len(r.CaptureBugs), r.RunnerOS)
		for _, f := range r.CaptureBugs {
			fmt.Printf("  %s missing %s (declared)\n", f.ELF, f.Name)
		}
	}
}
