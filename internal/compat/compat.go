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

// DefaultIniValues returns the ini key/value pairs that shivammathur/setup-php@v2
// sets on Linux runners by default, before any user-supplied ini-values. The
// caller merges the user values over the top so users can still override.
//
// Only unconditional defaults are returned here. Version-conditional or
// extension-tied defaults (e.g. xdebug.mode=coverage, opcache.*) are applied
// at compose time by their respective handlers, not by this function.
//
// Data source: docs/compat-matrix.md §2.1; mirrored in testdata/default_ini_values.golden.
func DefaultIniValues(phpVersion string) map[string]string {
	// Version-independent today. If a version shift emerges, dispatch on
	// phpVersion (e.g. switch major/minor).
	_ = phpVersion
	return map[string]string{
		"date.timezone": "UTC",
		"memory_limit":  "-1",
	}
}
