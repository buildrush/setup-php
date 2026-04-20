package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// CommitOpts configures CommitLockfile.
type CommitOpts struct {
	LockfilePath string // absolute or relative path to the lockfile to commit
	BranchRef    string // remote branch to push to (no refs/heads/ prefix)
	RepoDir      string // git work tree; default "." (current directory)
	Message      string // commit message body
	ActorName    string // git author + committer name
	ActorEmail   string // git author + committer email
}

// CommitLockfile stages LockfilePath, exits successfully if there is no diff,
// otherwise commits with the provided identity and pushes to BranchRef with
// --force-with-lease. All git invocations run in RepoDir.
func CommitLockfile(opts CommitOpts) error {
	dir := opts.RepoDir
	if dir == "" {
		dir = "."
	}

	if err := runGit(dir, nil, "add", "--", opts.LockfilePath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// `git diff --cached --quiet` exits 0 when there is no staged diff,
	// exit 1 when there is. Any other exit is an error.
	quiet := exec.Command("git", "-C", dir, "diff", "--cached", "--quiet", "--", opts.LockfilePath)
	err := quiet.Run()
	if err == nil {
		return nil // no diff; nothing to commit
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		return fmt.Errorf("git diff --cached: %w", err)
	}

	// Commit with explicit identity so we don't inherit stale local git config.
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME="+opts.ActorName,
		"GIT_AUTHOR_EMAIL="+opts.ActorEmail,
		"GIT_COMMITTER_NAME="+opts.ActorName,
		"GIT_COMMITTER_EMAIL="+opts.ActorEmail,
	)
	if err := runGit(dir, env, "commit", "-m", opts.Message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if err := runGit(dir, nil, "push", "--force-with-lease", "origin", "HEAD:"+opts.BranchRef); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

// runGit invokes `git -C dir args...` with stdout/stderr captured into err.
func runGit(dir string, env []string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, buf.String())
	}
	return nil
}
