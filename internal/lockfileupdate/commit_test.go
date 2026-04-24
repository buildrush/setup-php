package lockfileupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mustRun executes a command and fails the test if it returns an error.
func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// setupRepoFixture creates a bare "origin" repo and a work clone with an
// initial commit. Returns the work-clone path.
func setupRepoFixture(t *testing.T) string {
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

	run(root, "git", "init", "--bare", "-b", "main", origin)
	run(root, "git", "clone", origin, work)
	run(work, "git", "config", "user.email", "t@example.com")
	run(work, "git", "config", "user.name", "t")
	run(work, "git", "config", "commit.gpgsign", "false")
	run(work, "sh", "-c", "echo initial > bundles.lock && git add bundles.lock && git commit -m initial && git push -u origin main")
	return work
}

func TestCommitLockfile_NoDiff_ExitsZero(t *testing.T) {
	work := setupRepoFixture(t)

	opts := CommitOpts{
		LockfilePath: filepath.Join(work, "bundles.lock"),
		BranchRef:    "main",
		RepoDir:      work,
		Message:      "chore(lock): test",
		ActorName:    "bot",
		ActorEmail:   "bot@example.com",
	}
	if err := CommitLockfile(opts); err != nil {
		t.Fatalf("expected nil on no-diff, got %v", err)
	}

	// Verify no new commit was created.
	cmd := exec.Command("git", "-C", work, "rev-list", "--count", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	if got := string(out); got != "1\n" {
		t.Fatalf("expected 1 commit after no-diff commit, got %q", got)
	}
}

func TestCommitLockfile_Diff_CommitsAndPushes(t *testing.T) {
	work := setupRepoFixture(t)

	// Mutate the lockfile so there is a real diff to commit.
	if err := os.WriteFile(filepath.Join(work, "bundles.lock"), []byte("updated"), 0o644); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}

	opts := CommitOpts{
		LockfilePath: filepath.Join(work, "bundles.lock"),
		BranchRef:    "main",
		RepoDir:      work,
		Message:      "chore(lock): update bundles.lock from pipeline 999",
		ActorName:    "github-actions[bot]",
		ActorEmail:   "41898282+github-actions[bot]@users.noreply.github.com",
	}
	if err := CommitLockfile(opts); err != nil {
		t.Fatalf("CommitLockfile: %v", err)
	}

	// Verify a new commit exists on main in the bare origin.
	originDir := filepath.Join(filepath.Dir(work), "origin.git")
	out, err := exec.Command("git", "-C", originDir, "log", "-1", "--format=%s", "main").Output()
	if err != nil {
		t.Fatalf("git log on origin: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != opts.Message {
		t.Fatalf("origin main top commit = %q, want %q", got, opts.Message)
	}

	// Verify author/committer identity.
	out, err = exec.Command("git", "-C", originDir, "log", "-1", "--format=%an <%ae>", "main").Output()
	if err != nil {
		t.Fatalf("git log author: %v", err)
	}
	want := opts.ActorName + " <" + opts.ActorEmail + ">"
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("author = %q, want %q", got, want)
	}
}

func TestCommitLockfile_LeaseRefusal(t *testing.T) {
	work := setupRepoFixture(t)
	originDir := filepath.Join(filepath.Dir(work), "origin.git")

	// Make the origin advance beyond what the work tree saw.
	secondClone := filepath.Join(filepath.Dir(work), "work2")
	mustRun(t, ".", "git", "clone", originDir, secondClone)
	mustRun(t, secondClone, "git", "config", "user.email", "u@example.com")
	mustRun(t, secondClone, "git", "config", "user.name", "u")
	mustRun(t, secondClone, "git", "config", "commit.gpgsign", "false")
	mustRun(t, secondClone, "sh", "-c", "echo concurrent > bundles.lock && git add . && git commit -m concurrent && git push origin main")

	// Now the work tree's origin/main is stale. Attempting force-with-lease must fail.
	if err := os.WriteFile(filepath.Join(work, "bundles.lock"), []byte("ours"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := CommitOpts{
		LockfilePath: filepath.Join(work, "bundles.lock"),
		BranchRef:    "main",
		RepoDir:      work,
		Message:      "chore(lock): racy",
		ActorName:    "bot",
		ActorEmail:   "bot@example.com",
	}
	err := CommitLockfile(opts)
	if err == nil {
		t.Fatal("expected push to fail under stale --force-with-lease, got nil")
	}
}
