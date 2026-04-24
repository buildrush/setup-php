package gcbundles

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupRepoWithLockfiles creates a bare origin repo with two branches and one
// annotated tag, each carrying a different bundles.lock. Returns the work
// tree path the caller should use when invoking the enumerator.
func setupRepoWithLockfiles(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	run := func(dir, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	write := func(dir, file, content string) {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	run(root, "git", "init", "--bare", "-b", "main", origin)
	run(root, "git", "clone", origin, work)
	run(work, "git", "config", "user.email", "t@example.com")
	run(work, "git", "config", "user.name", "t")
	run(work, "git", "config", "commit.gpgsign", "false")

	// main has digest A
	write(work, "bundles.lock", `{"schema_version":2,"bundles":{"k1":{"digest":"sha256:A","spec_hash":"sha256:hA"}}}`)
	run(work, "git", "add", "bundles.lock")
	run(work, "git", "commit", "-m", "main")
	run(work, "git", "push", "-u", "origin", "main")

	// feature branch adds digest B
	run(work, "git", "checkout", "-b", "feat/x")
	write(work, "bundles.lock", `{"schema_version":2,"bundles":{"k1":{"digest":"sha256:B","spec_hash":"sha256:hB"}}}`)
	run(work, "git", "commit", "-am", "feat")
	run(work, "git", "push", "-u", "origin", "feat/x")

	// Create C on a detached HEAD, tag v0.1.0, and push the tag.
	// This keeps the commit reachable only through the tag, not through any branch.
	run(work, "git", "checkout", "--detach", "main")
	write(work, "bundles.lock", `{"schema_version":2,"bundles":{"k1":{"digest":"sha256:C","spec_hash":"sha256:hC"}}}`)
	run(work, "git", "add", "bundles.lock")
	run(work, "git", "commit", "-m", "release")
	run(work, "git", "tag", "-a", "v0.1.0", "-m", "release 0.1.0")
	run(work, "git", "push", "origin", "v0.1.0")

	return work
}

func TestEnumerateReferencedDigests_UnionsAllRefs(t *testing.T) {
	work := setupRepoWithLockfiles(t)

	got, err := EnumerateReferencedDigests(work, []string{"v0.1.0"})
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}

	for _, want := range []string{"sha256:A", "sha256:B", "sha256:C"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %s in referenced set; got %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected 3 digests, got %d: %v", len(got), got)
	}
}
