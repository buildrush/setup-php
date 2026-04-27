package testsuite

import (
	"context"
	"errors"
	"fmt"

	"github.com/buildrush/setup-php/internal/layoutlockfile"
)

// InternalMain is the entry point for `phpup internal …` maintainer-only
// subcommands. These are sibling commands to `phpup build` and `phpup test`
// but are separated under the `internal` umbrella because they're meant to
// be invoked by the outer tooling (e.g. `phpup test` re-enters via
// `phpup internal test-cell` inside the per-cell container, and `smoke-action`
// in CI invokes `phpup internal lockfile-from-layout` to synthesize a
// PR-local lockfile from a downloaded oci-layout artifact), not driven
// directly by users.
func InternalMain(args []string) error {
	if len(args) == 0 {
		return errors.New("phpup internal: usage: phpup internal <subcommand> [flags]")
	}
	switch args[0] {
	case "test-cell":
		return RunTestCell(context.Background(), args[1:])
	case "lockfile-from-layout":
		return layoutlockfile.MainCLI(args[1:])
	default:
		return fmt.Errorf("phpup internal: unknown subcommand %q", args[0])
	}
}
