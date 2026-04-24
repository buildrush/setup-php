package gcbundles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// EnumerateReferencedDigests returns the union of every bundles[*].digest
// across bundles.lock on every remote branch and every given release tag.
// Missing or unparseable lockfiles on a ref are treated as empty — a stale
// branch without bundles.lock should not block GC of its ancestor branches.
//
// gitRepo is a path to a working tree (any clone works; remote refs are
// queried via `git show origin/<branch>:bundles.lock`). releaseTags is the
// caller-supplied list of released tag names; we decouple tag discovery
// from enumeration so callers can plug in `gh release list` or fixtures.
func EnumerateReferencedDigests(gitRepo string, releaseTags []string) (map[string]struct{}, error) {
	referenced := map[string]struct{}{}

	branches, err := listRemoteBranches(gitRepo)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	refs := make([]string, 0, len(branches)+len(releaseTags))
	for _, b := range branches {
		refs = append(refs, "refs/remotes/origin/"+b)
	}
	for _, t := range releaseTags {
		refs = append(refs, "refs/tags/"+t)
	}

	for _, ref := range refs {
		digests, err := extractDigestsAt(gitRepo, ref)
		if err != nil {
			// Ref may not have bundles.lock (e.g. ancient branch). Skip.
			continue
		}
		for _, d := range digests {
			referenced[d] = struct{}{}
		}
	}

	return referenced, nil
}

func listRemoteBranches(gitRepo string) ([]string, error) {
	cmd := exec.Command("git", "-C", gitRepo, "ls-remote", "--heads", "origin")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-remote: %w\n%s", err, out.String())
	}
	var names []string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "<sha>\trefs/heads/<name>"
		parts := strings.SplitN(line, "refs/heads/", 2)
		if len(parts) == 2 && parts[1] != "" {
			names = append(names, parts[1])
		}
	}
	return names, nil
}

func extractDigestsAt(gitRepo, ref string) ([]string, error) {
	cmd := exec.Command("git", "-C", gitRepo, "show", ref+":bundles.lock")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	// Parse just enough to reach digest fields; keep the shape permissive so
	// schema evolution doesn't break GC.
	var probe struct {
		Bundles map[string]struct {
			Digest string `json:"digest"`
		} `json:"bundles"`
	}
	if err := json.Unmarshal(out.Bytes(), &probe); err != nil {
		return nil, fmt.Errorf("parse %s:bundles.lock: %w", ref, err)
	}
	digests := make([]string, 0, len(probe.Bundles))
	for _, b := range probe.Bundles {
		if b.Digest != "" {
			digests = append(digests, b.Digest)
		}
	}
	return digests, nil
}
