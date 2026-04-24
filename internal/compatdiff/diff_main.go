// cmd/compat-diff/main.go
package compatdiff

import (
	"flag"
	"fmt"
	"os"
	"regexp"
)

const (
	exitMatch     = 0
	exitDiff      = 1
	exitMalformed = 2
)

type cliArgs struct {
	ours      string
	theirs    string
	allowlist string
	fixture   string
}

var fixtureNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validFixtureName(name string) bool {
	return fixtureNameRe.MatchString(name)
}

func parseFlags(args []string, stderr *os.File) (parsed cliArgs, exitCode int) {
	fs := flag.NewFlagSet("compat-diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&parsed.ours, "ours", "", "path to our probe JSON")
	fs.StringVar(&parsed.theirs, "theirs", "", "path to theirs probe JSON")
	fs.StringVar(&parsed.allowlist, "allowlist", "", "path to compat-matrix.md holding deviations block")
	fs.StringVar(&parsed.fixture, "fixture", "", "fixture name (used for allowlist filtering)")
	if err := fs.Parse(args); err != nil {
		return parsed, exitMalformed
	}
	missing := parsed.ours == "" || parsed.theirs == "" || parsed.allowlist == "" || parsed.fixture == ""
	if missing {
		_, _ = fmt.Fprintln(stderr, "usage: compat-diff --ours <path> --theirs <path> --allowlist <path> --fixture <name>")
		return parsed, exitMalformed
	}
	if !validFixtureName(parsed.fixture) {
		_, _ = fmt.Fprintf(stderr, "compat-diff: invalid --fixture %q (must match [a-z0-9][a-z0-9_-]{0,63})\n", parsed.fixture)
		return parsed, exitMalformed
	}
	return parsed, 0
}

func run(args cliArgs, stdout, stderr *os.File) int {
	ours, err := readProbe(args.ours)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "::error::fixture=%s %v\n", args.fixture, err)
		return exitMalformed
	}
	theirs, err := readProbe(args.theirs)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "::error::fixture=%s %v\n", args.fixture, err)
		return exitMalformed
	}
	al, err := loadAllowlist(args.allowlist)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "::error::fixture=%s %v\n", args.fixture, err)
		return exitMalformed
	}
	diffs := diffProbes(&ours, &theirs, al, args.fixture)
	if len(diffs) == 0 {
		_, _ = fmt.Fprintf(stdout, "compat-diff: fixture=%s OK (0 deviations)\n", args.fixture)
		return exitMatch
	}
	_, _ = fmt.Fprintf(stdout, "compat-diff: fixture=%s FAIL (%d unexplained deviation(s))\n", args.fixture, len(diffs))
	for _, d := range diffs {
		_, _ = fmt.Fprintf(stderr, "::error::fixture=%s path=%s ours=%s theirs=%s\n",
			args.fixture, d.Path, d.Ours, d.Theirs)
	}
	return exitDiff
}

// ExitCode is returned by Main so the phpup dispatcher can preserve the
// pre-refactor cmd/compat-diff exit convention:
//
//	0 = match (no deviations)
//	1 = deviation(s) found
//	2 = usage / I/O / malformed-input error
type ExitCode int

const (
	ExitMatch     ExitCode = exitMatch
	ExitDiff      ExitCode = exitDiff
	ExitMalformed ExitCode = exitMalformed
)

// Main is the entry point for `phpup compat-diff`. args is everything after
// the subcommand token. Returns an ExitCode the phpup dispatcher converts to
// os.Exit; byte-identical stdout/stderr to the retired cmd/compat-diff
// binary for the same inputs.
func Main(args []string) ExitCode {
	parsed, code := parseFlags(args, os.Stderr)
	if code != 0 {
		return ExitCode(code)
	}
	return ExitCode(run(parsed, os.Stdout, os.Stderr))
}
