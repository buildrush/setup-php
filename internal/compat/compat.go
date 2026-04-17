// Package compat is the single source of truth for drop-in compatibility with
// shivammathur/setup-php@v2. All data in this package is derived from the
// audit documented in docs/compat-matrix.md and should be updated via
// deliberate PRs that bump the pinned reference.
package compat

import (
	"fmt"
	"strings"
)

// normalizeExtensionName converts a raw `php -m` extension name to the
// user-facing identifier used in `extensions:` input and throughout the rest
// of the codebase (lowercase, with the one historical alias fixup).
//
// Rules:
//   - Lowercase the input.
//   - "Zend OPcache" (lowercased to "zend opcache") → "opcache", matching the
//     identifier users type and shivammathur/setup-php@v2's convention.
//   - All other inputs: just lowercase.
func normalizeExtensionName(raw string) string {
	lower := strings.ToLower(raw)
	if lower == "zend opcache" {
		return "opcache"
	}
	return lower
}

// minorOf returns the X.Y prefix of a PHP version string. "8.4.6" → "8.4",
// "8.4" → "8.4", anything without a second dot is returned unchanged.
// Used to normalize user-supplied PHP versions before looking up per-minor
// compat data (which is keyed by X.Y only).
func minorOf(phpVersion string) string {
	first := strings.IndexByte(phpVersion, '.')
	if first < 0 {
		return phpVersion
	}
	second := strings.IndexByte(phpVersion[first+1:], '.')
	if second < 0 {
		return phpVersion
	}
	return phpVersion[:first+1+second]
}

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

// BundledExtensions returns the set of extensions compiled in to (or bundled
// with) the shivammathur/setup-php@v2 Linux build for a given PHP version —
// i.e. what `php -m` reports after setup-php runs with an empty `extensions:`
// input, based on the Ondrej PPA build for that version. Returns nil for
// unknown versions.
//
// Names are returned as user-facing extension identifiers (lowercase,
// `Zend OPcache` → `opcache`), suitable for matching against `extensions:`
// input. The underlying golden files preserve the raw `php -m` casing for
// audit purposes; normalization happens here.
//
// The returned slice is a copy; callers may mutate it without affecting other
// callers.
//
// Data source: docs/compat-matrix.md §3; mirrored in testdata/bundled_extensions_<ver>.golden.
func BundledExtensions(phpVersion string) []string {
	var src []string
	switch minorOf(phpVersion) {
	case "8.1":
		src = bundled81
	case "8.2":
		src = bundled82
	case "8.3":
		src = bundled83
	case "8.4":
		src = bundled84
	case "8.5":
		src = bundled85
	default:
		return nil
	}
	out := make([]string, len(src))
	for i, name := range src {
		out[i] = normalizeExtensionName(name)
	}
	return out
}

// Per-version bundled extension lists. Order matches the native `php -m`
// output captured in the audit (see docs/compat-matrix.md §3.x and the
// matching testdata/bundled_extensions_<ver>.golden file).

var bundled81 = []string{
	"calendar",
	"Core",
	"ctype",
	"date",
	"exif",
	"FFI",
	"fileinfo",
	"filter",
	"ftp",
	"gettext",
	"hash",
	"iconv",
	"json",
	"libxml",
	"openssl",
	"pcntl",
	"pcre",
	"PDO",
	"Phar",
	"posix",
	"readline",
	"Reflection",
	"session",
	"shmop",
	"sockets",
	"sodium",
	"SPL",
	"standard",
	"sysvmsg",
	"sysvsem",
	"sysvshm",
	"tokenizer",
	"Zend OPcache",
	"zlib",
}

var bundled82 = []string{
	"calendar",
	"Core",
	"ctype",
	"date",
	"exif",
	"FFI",
	"fileinfo",
	"filter",
	"ftp",
	"gettext",
	"hash",
	"iconv",
	"json",
	"libxml",
	"openssl",
	"pcntl",
	"pcre",
	"PDO",
	"Phar",
	"posix",
	"random",
	"readline",
	"Reflection",
	"session",
	"shmop",
	"sockets",
	"sodium",
	"SPL",
	"standard",
	"sysvmsg",
	"sysvsem",
	"sysvshm",
	"tokenizer",
	"Zend OPcache",
	"zlib",
}

var bundled83 = []string{
	"calendar",
	"Core",
	"ctype",
	"date",
	"exif",
	"FFI",
	"fileinfo",
	"filter",
	"ftp",
	"gettext",
	"hash",
	"iconv",
	"json",
	"libxml",
	"openssl",
	"pcntl",
	"pcre",
	"PDO",
	"Phar",
	"posix",
	"random",
	"readline",
	"Reflection",
	"session",
	"shmop",
	"sockets",
	"sodium",
	"SPL",
	"standard",
	"sysvmsg",
	"sysvsem",
	"sysvshm",
	"tokenizer",
	"Zend OPcache",
	"zlib",
}

var bundled84 = []string{
	"calendar",
	"Core",
	"ctype",
	"date",
	"exif",
	"FFI",
	"fileinfo",
	"filter",
	"ftp",
	"gettext",
	"hash",
	"iconv",
	"json",
	"libxml",
	"openssl",
	"pcntl",
	"pcre",
	"PDO",
	"Phar",
	"posix",
	"random",
	"readline",
	"Reflection",
	"session",
	"shmop",
	"sockets",
	"sodium",
	"SPL",
	"standard",
	"sysvmsg",
	"sysvsem",
	"sysvshm",
	"tokenizer",
	"Zend OPcache",
	"zlib",
}

var bundled85 = []string{
	"calendar",
	"Core",
	"ctype",
	"date",
	"exif",
	"FFI",
	"fileinfo",
	"filter",
	"ftp",
	"gettext",
	"hash",
	"iconv",
	"json",
	"lexbor",
	"libxml",
	"openssl",
	"pcntl",
	"pcre",
	"PDO",
	"Phar",
	"posix",
	"random",
	"readline",
	"Reflection",
	"session",
	"shmop",
	"sockets",
	"sodium",
	"SPL",
	"standard",
	"sysvmsg",
	"sysvsem",
	"sysvshm",
	"tokenizer",
	"uri",
	"Zend OPcache",
	"zlib",
}
