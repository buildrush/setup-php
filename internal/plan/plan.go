package plan

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type Plan struct {
	PHPVersion        string
	Extensions        []string
	ExtensionsExclude []string
	ExtensionsReset   bool
	IniValues         []IniValue
	Coverage          CoverageDriver
	Tools             []string
	ThreadSafety      string
	OS                string
	Arch              string
	FailFast          bool
	Update            bool
	IniFile           string
	Debug             bool
	Verbose           bool
}

type IniValue struct {
	Key   string
	Value string
}

type CoverageDriver string

const (
	CoverageNone   CoverageDriver = "none"
	CoverageXdebug CoverageDriver = "xdebug"
	CoveragePCOV   CoverageDriver = "pcov"
)

func FromEnv() (*Plan, error) {
	p := &Plan{
		PHPVersion:   envOrDefault("INPUT_PHP-VERSION", "8.4"),
		Coverage:     CoverageDriver(envOrDefault("INPUT_COVERAGE", "none")),
		ThreadSafety: envOrDefault("INPUT_PHPTS", "nts"),
		OS:           normalizeOS(os.Getenv("RUNNER_OS")),
		Arch:         normalizeArch(os.Getenv("RUNNER_ARCH")),
		FailFast:     os.Getenv("INPUT_FAIL-FAST") == "true",
		Update:       os.Getenv("INPUT_UPDATE") == "true",
		IniFile:      envOrDefault("INPUT_INI-FILE", "production"),
		Debug:        os.Getenv("INPUT_DEBUG") == "true",
		Verbose:      os.Getenv("INPUT_VERBOSE") == "true",
	}

	if versionFile := os.Getenv("INPUT_PHP-VERSION-FILE"); versionFile != "" && p.PHPVersion == "8.4" {
		if v, err := ParsePHPVersionFile(versionFile); err == nil {
			p.PHPVersion = v
		}
	}

	incl, excl, reset := ParseExtensions(os.Getenv("INPUT_EXTENSIONS"))
	p.Extensions = incl
	p.ExtensionsExclude = excl
	p.ExtensionsReset = reset

	if raw := os.Getenv("INPUT_INI-VALUES"); raw != "" {
		vals, err := ParseIniValues(raw)
		if err != nil {
			return nil, fmt.Errorf("parse ini-values: %w", err)
		}
		p.IniValues = vals
	}

	if raw := os.Getenv("INPUT_TOOLS"); raw != "" {
		p.Tools = parseCSV(raw)
	}

	return p, nil
}

// ParseExtensions parses shivammathur/setup-php@v2's extensions syntax.
// Recognized tokens:
//   - bare name (e.g. "redis") → include
//   - ":name" → exclude
//   - "none" → reset both include and exclude to empty; subsequent tokens process normally
//
// Output is sorted, deduplicated, and lower-cased.
func ParseExtensions(raw string) (include, exclude []string, reset bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, false
	}

	seenIncl := map[string]bool{}
	seenExcl := map[string]bool{}

	for tok := range strings.SplitSeq(raw, ",") {
		tok = strings.TrimSpace(strings.ToLower(tok))
		switch {
		case tok == "":
			// skip
		case tok == "none":
			include = nil
			exclude = nil
			seenIncl = map[string]bool{}
			seenExcl = map[string]bool{}
			reset = true
		case strings.HasPrefix(tok, ":"):
			name := strings.TrimPrefix(tok, ":")
			if name == "" || seenExcl[name] {
				continue
			}
			seenExcl[name] = true
			exclude = append(exclude, name)
		default:
			if seenIncl[tok] {
				continue
			}
			seenIncl[tok] = true
			include = append(include, tok)
		}
	}

	sort.Strings(include)
	sort.Strings(exclude)
	return include, exclude, reset
}

func ParseIniValues(raw string) ([]IniValue, error) {
	var vals []IniValue
	for pair := range strings.SplitSeq(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ini value %q: expected key=value", pair)
		}
		vals = append(vals, IniValue{
			Key:   strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}
	return vals, nil
}

func ParsePHPVersionFile(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ApplyCoverage adds the requested coverage driver (xdebug or pcov) to the
// extensions list so it is resolved, fetched, and composed like any other
// extension. "none" is a no-op.
func (p *Plan) ApplyCoverage() {
	var driver string
	switch p.Coverage {
	case CoverageXdebug:
		driver = "xdebug"
	case CoveragePCOV:
		driver = "pcov"
	default:
		return
	}
	if slices.Contains(p.Extensions, driver) {
		return
	}
	p.Extensions = append(p.Extensions, driver)
	sort.Strings(p.Extensions)
}

func (p *Plan) Hash() string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "php:%s\n", p.PHPVersion)
	_, _ = fmt.Fprintf(h, "os:%s\n", p.OS)
	_, _ = fmt.Fprintf(h, "arch:%s\n", p.Arch)
	_, _ = fmt.Fprintf(h, "ts:%s\n", p.ThreadSafety)
	_, _ = fmt.Fprintf(h, "exts:%s\n", strings.Join(p.Extensions, ","))
	_, _ = fmt.Fprintf(h, "excl:%s\n", strings.Join(p.ExtensionsExclude, ","))
	_, _ = fmt.Fprintf(h, "reset:%t\n", p.ExtensionsReset)
	_, _ = fmt.Fprintf(h, "coverage:%s\n", p.Coverage)
	for _, iv := range p.IniValues {
		_, _ = fmt.Fprintf(h, "ini:%s=%s\n", iv.Key, iv.Value)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func normalizeOS(s string) string {
	switch strings.ToLower(s) {
	case "linux":
		return "linux"
	case "macos", "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return strings.ToLower(s)
	}
}

func normalizeArch(s string) string {
	switch strings.ToLower(s) {
	case "x64", "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	default:
		return strings.ToLower(s)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseCSV(raw string) []string {
	var result []string
	for s := range strings.SplitSeq(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
