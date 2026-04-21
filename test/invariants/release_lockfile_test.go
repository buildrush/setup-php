//go:build invariants
// +build invariants

// Package invariants holds cross-release assertions that run in release-please
// and nightly workflows — separated from regular tests so `go test ./...` stays
// fast and hermetic.
package invariants

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

// TestReleasedLockfilesAreReachable ensures every digest referenced by any
// published release's bundles.lock still resolves on GHCR. If this fails,
// a released @vX.Y.Z action version would produce a broken install,
// regressing product-vision §16 reproducibility.
func TestReleasedLockfilesAreReachable(t *testing.T) {
	tags := listReleasedTags(t)
	if len(tags) == 0 {
		t.Skip("no published releases — invariant vacuously holds")
	}

	for _, tag := range tags {
		tag := tag
		t.Run(tag, func(t *testing.T) {
			bundles := parseLockfileAt(t, tag)
			if len(bundles) == 0 {
				t.Skipf("%s has no bundles.lock entries", tag)
			}
			for key, digest := range bundles {
				if !digestReachable(t, key, digest) {
					t.Errorf("%s: digest %s (key=%s) NOT reachable on GHCR", tag, digest, key)
				}
			}
		})
	}
}

func listReleasedTags(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("gh", "release", "list", "--json", "tagName", "--limit", "500").Output()
	if err != nil {
		t.Fatalf("gh release list: %v", err)
	}
	var releases []struct {
		TagName string `json:"tagName"`
	}
	if err := json.Unmarshal(out, &releases); err != nil {
		t.Fatalf("parse releases: %v", err)
	}
	tags := make([]string, 0, len(releases))
	for _, r := range releases {
		tags = append(tags, r.TagName)
	}
	return tags
}

func parseLockfileAt(t *testing.T, tag string) map[string]string {
	t.Helper()
	out, err := exec.Command("git", "show", "refs/tags/"+tag+":bundles.lock").Output()
	if err != nil {
		return nil
	}
	var probe struct {
		Bundles map[string]struct {
			Digest string `json:"digest"`
		} `json:"bundles"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Logf("%s: parse bundles.lock: %v", tag, err)
		return nil
	}
	result := map[string]string{}
	for k, v := range probe.Bundles {
		if v.Digest != "" {
			result[k] = v.Digest
		}
	}
	return result
}

// digestReachable HEADs the manifest at ghcr.io. Keys like
// "ext:xdebug:3.5.1:8.4:linux:x86_64:nts" map to
// "ghcr.io/<org>/php-ext-xdebug@<digest>".
func digestReachable(t *testing.T, key, digest string) bool {
	t.Helper()
	org := "buildrush"
	var pkg string
	switch {
	case strings.HasPrefix(key, "php:"):
		pkg = "php-core"
	case strings.HasPrefix(key, "ext:"):
		fields := strings.Split(key, ":")
		if len(fields) < 2 {
			t.Errorf("malformed key %q", key)
			return false
		}
		pkg = "php-ext-" + fields[1]
	default:
		t.Errorf("unknown key prefix %q", key)
		return false
	}
	url := "https://ghcr.io/v2/" + org + "/" + pkg + "/manifests/" + digest

	tokenURL := "https://ghcr.io/token?scope=repository:" + org + "/" + pkg + ":pull"
	tokenResp, err := http.Get(tokenURL)
	if err != nil {
		t.Logf("token fetch %s: %v", tokenURL, err)
		return false
	}
	defer tokenResp.Body.Close()
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tok); err != nil {
		t.Logf("decode token: %v", err)
		return false
	}

	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("HEAD %s: %v", url, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Logf("HEAD %s: %d %s", url, resp.StatusCode, buf.String())
		return false
	}
	return true
}
