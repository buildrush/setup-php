// Package compat is the single source of truth for drop-in compatibility with
// shivammathur/setup-php@v2. All data in this package is derived from the
// audit documented in docs/compat-matrix.md and should be updated via
// deliberate PRs that bump the pinned reference.
package compat

import "fmt"

// UnimplementedInputWarning returns the canonical one-line warning emitted when
// a user sets an input that buildrush cannot implement given its architecture.
// The text starts with GitHub Actions' "::warning::" prefix so runners fold it.
func UnimplementedInputWarning(inputName, value string) string {
	if inputName == "update" {
		return fmt.Sprintf(
			"::warning::input 'update=%s' is not supported by buildrush/setup-php (prebuilt bundles are already up to date); ignoring",
			value,
		)
	}
	return fmt.Sprintf(
		"::warning::input '%s=%s' is not supported by buildrush/setup-php; ignoring",
		inputName, value,
	)
}
