package testsuite

import (
	"context"
	"errors"
	"fmt"
)

// InternalMain is the entry point for `phpup internal …` maintainer-only
// subcommands. These are sibling commands to `phpup build` and `phpup test`
// but are separated under the `internal` umbrella because they're meant to
// be invoked by the outer tooling (e.g. `phpup test` re-enters via
// `phpup internal test-cell` inside the per-cell container), not driven
// directly by users.
func InternalMain(args []string) error {
	if len(args) == 0 {
		return errors.New("phpup internal: usage: phpup internal <subcommand> [flags]")
	}
	switch args[0] {
	case "test-cell":
		return testCellStub(context.Background(), args[1:])
	default:
		return fmt.Errorf("phpup internal: unknown subcommand %q", args[0])
	}
}

// testCellStub is a placeholder until PR 3 Task 3 replaces it with the
// real container-side fixture runner. Same pattern as PR 2's BuildExt
// stub: errors with a clear message so the dispatch compiles end-to-end
// and the outer `phpup test` invocation can wire up the docker run
// without waiting on the inner implementation.
func testCellStub(_ context.Context, _ []string) error {
	return errors.New("phpup internal test-cell: not yet implemented; will land in the next commit")
}
