// cmd/compat-diff/main.go
package main

import (
	"flag"
	"fmt"
	"os"
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

func parseFlags(args []string, stderr *os.File) (cliArgs, int) {
	fs := flag.NewFlagSet("compat-diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var a cliArgs
	fs.StringVar(&a.ours, "ours", "", "path to our probe JSON")
	fs.StringVar(&a.theirs, "theirs", "", "path to theirs probe JSON")
	fs.StringVar(&a.allowlist, "allowlist", "", "path to compat-matrix.md holding deviations block")
	fs.StringVar(&a.fixture, "fixture", "", "fixture name (used for allowlist filtering)")
	if err := fs.Parse(args); err != nil {
		return a, exitMalformed
	}
	missing := a.ours == "" || a.theirs == "" || a.allowlist == "" || a.fixture == ""
	if missing {
		fmt.Fprintln(stderr, "usage: compat-diff --ours <path> --theirs <path> --allowlist <path> --fixture <name>")
		return a, exitMalformed
	}
	return a, 0
}

func main() {
	args, code := parseFlags(os.Args[1:], os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
	_ = args
	os.Exit(exitMatch)
}
