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

// BaseIniFileName maps the user-facing `ini-file` input to the upstream PHP
// source file name stored under share/php/ini/ in the core bundle. Returns
// ("", "") for "none" (effective php.ini is written empty). Any value that
// isn't one of the five v2 aliases falls back to production and returns a
// ::warning:: line.
//
// Data source: docs/compat-matrix.md §1.1 note on parseIniFile (src/utils.ts L88-97).
func BaseIniFileName(iniFile string) (filename, warning string) {
	switch iniFile {
	case "", "production", "php.ini-production":
		return "php.ini-production", ""
	case "development", "php.ini-development":
		return "php.ini-development", ""
	case "none":
		return "", ""
	default:
		return "php.ini-production", fmt.Sprintf(
			"::warning::input 'ini-file=%s' is not a recognized value; falling back to production",
			iniFile,
		)
	}
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

// OurBuildBundledExtras returns extensions our own PHP core bundle compiles
// in beyond v2's baseline (BundledExtensions). Reflects the current builder's
// ./configure flag set. Merge with BundledExtensions when the runtime needs
// the full "preloaded by this bundle" list.
//
// See docs/superpowers/specs/2026-04-17-phase2-t12-handoff.md for the planned
// builder alignment; this list is intentionally a superset of v2's baseline
// until that work lands.
func OurBuildBundledExtras(phpVersion string) []string {
	switch minorOf(phpVersion) {
	case "8.4":
		// Matches the --enable-*/--with-* flags in builders/linux/build-php.sh
		// minus anything already in v2's baseline.
		return []string{
			"mbstring", "curl", "intl", "zip",
			"pdo_mysql", "pdo_sqlite", "sqlite3", "pdo_pgsql", "pgsql",
			"bcmath", "soap", "gd",
			"xml", "dom", "simplexml", "xmlreader", "xmlwriter",
		}
	default:
		return nil
	}
}

// XdebugIniFragment returns the ini key/value pairs from v2's xdebug.ini when
// xdebug3 is active for the requested PHP version (matches regex 7.[2-4]|8.[0-9]
// per docs/compat-matrix.md §2.2). Returns nil for any other version.
//
// Deliberate divergence from v2's mechanism: v2 unconditionally writes this
// fragment for every matching PHP version, even when xdebug is not installed
// (PHP silently ignores ini keys for unloaded extensions). We take the
// stricter approach and have the caller apply this map only when xdebug is
// present in the resolved extension set. The observable end-state — the
// effective ini value when xdebug *is* loaded — is identical; only the file
// on disk differs.
func XdebugIniFragment(phpVersion string) map[string]string {
	m := minorOf(phpVersion)
	if !xdebug3Supported(m) {
		return nil
	}
	return map[string]string{"xdebug.mode": "coverage"}
}

func xdebug3Supported(minor string) bool {
	// compat-matrix §2.2: xdebug3_versions = 7.[2-4]|8.[0-9]
	switch minor {
	case "7.2", "7.3", "7.4":
		return true
	case "8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6", "8.7", "8.8", "8.9":
		return true
	}
	return false
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
