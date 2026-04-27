package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestUnimplementedInputWarning(t *testing.T) {
	tests := []struct {
		name, input, value, want string
	}{
		{
			name:  "update true",
			input: "update",
			value: "true",
			want:  "::warning::input 'update=true' is not supported by buildrush/setup-php (prebuilt bundles are already up to date); ignoring",
		},
		{
			name:  "unknown input",
			input: "foo",
			value: "bar",
			want:  "::warning::input 'foo=bar' is not supported by buildrush/setup-php; ignoring",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnimplementedInputWarning(tt.input, tt.value)
			if got != tt.want {
				t.Errorf("UnimplementedInputWarning(%q, %q) = %q, want %q", tt.input, tt.value, got, tt.want)
			}
		})
	}
}

func TestDefaultIniValuesMatchesGolden(t *testing.T) {
	// The golden file pins x86_64's expected keys. aarch64 branch has its
	// own targeted cases in TestDefaultIniValues_PHP8xOpcacheJIT below.
	got := DefaultIniValues("8.4", "x86_64")

	want := map[string]string{}
	for _, line := range readGoldenLines(t, "default_ini_values.golden") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("bad golden line: %q", line)
		}
		want[parts[0]] = parts[1]
	}

	if len(got) != len(want) {
		t.Fatalf("DefaultIniValues len = %d, golden len = %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("DefaultIniValues[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestBundledExtensionsMatchGolden(t *testing.T) {
	versions := []string{"8.1", "8.2", "8.3", "8.4", "8.5"}
	for _, v := range versions {
		t.Run(v, func(t *testing.T) {
			got := BundledExtensions(v)
			raw := readGoldenLines(t, fmt.Sprintf("bundled_extensions_%s.golden", v))
			// Golden files preserve raw `php -m` casing for audit; apply the
			// same normalization BundledExtensions does before comparing, so
			// the test asserts the normalization is stable without coupling
			// the golden to the user-facing identifier form.
			want := make([]string, len(raw))
			for i, line := range raw {
				want[i] = normalizeExtensionName(line)
			}
			gotSorted := append([]string(nil), got...)
			wantSorted := append([]string(nil), want...)
			sort.Strings(gotSorted)
			sort.Strings(wantSorted)
			if len(gotSorted) != len(wantSorted) {
				t.Fatalf("BundledExtensions(%q) len=%d, want %d\n got: %v\nwant: %v",
					v, len(gotSorted), len(wantSorted), gotSorted, wantSorted)
			}
			for i := range wantSorted {
				if gotSorted[i] != wantSorted[i] {
					t.Errorf("BundledExtensions(%q)[%d] = %q, want %q", v, i, gotSorted[i], wantSorted[i])
				}
			}
		})
	}
}

func TestNormalizeExtensionName(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"zend opcache native", "Zend OPcache", "opcache"},
		{"zend opcache lowercase", "zend opcache", "opcache"},
		{"PDO", "PDO", "pdo"},
		{"Core", "Core", "core"},
		{"mbstring idempotent", "mbstring", "mbstring"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeExtensionName(tt.in)
			if got != tt.want {
				t.Errorf("normalizeExtensionName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBundledExtensionsAcceptsFullTriple(t *testing.T) {
	// User may supply "8.4.6" (from php-version-file or explicit input).
	// BundledExtensions should normalize to the minor and return the same
	// data as for "8.4".
	minor := BundledExtensions("8.4")
	triple := BundledExtensions("8.4.6")
	if len(triple) == 0 {
		t.Fatalf("BundledExtensions(8.4.6) returned nil; expected to normalize to 8.4")
	}
	if len(minor) != len(triple) {
		t.Fatalf("BundledExtensions(8.4) len=%d, BundledExtensions(8.4.6) len=%d", len(minor), len(triple))
	}
	for i := range minor {
		if minor[i] != triple[i] {
			t.Errorf("diff at [%d]: minor=%q, triple=%q", i, minor[i], triple[i])
		}
	}
}

func TestOurBuildBundledExtras(t *testing.T) {
	// 8.4 must include the configure-flag extras so runtime resolution treats
	// them as preloaded (until T12 aligns our build with v2's slim baseline).
	got := OurBuildBundledExtras("8.4")
	for _, want := range []string{"mbstring", "curl", "intl", "zip", "bcmath", "gd"} {
		if !slices.Contains(got, want) {
			t.Errorf("OurBuildBundledExtras(8.4) missing %q; got %v", want, got)
		}
	}
	// Unknown/untracked version returns nil.
	if got := OurBuildBundledExtras("7.4"); got != nil {
		t.Errorf("OurBuildBundledExtras(7.4) = %v, want nil", got)
	}
	// Full-triple input normalizes to minor.
	if got := OurBuildBundledExtras("8.4.6"); !slices.Contains(got, "curl") {
		t.Errorf("OurBuildBundledExtras(8.4.6) did not normalize to 8.4; got %v", got)
	}
}

func TestMinorOf(t *testing.T) {
	tests := map[string]string{
		"8.4":       "8.4",
		"8.4.6":     "8.4",
		"8.4.20-rc": "8.4",
		"8.10":      "8.10",
		"8.10.0":    "8.10",
		"":          "",
		"8":         "8",
	}
	for in, want := range tests {
		if got := minorOf(in); got != want {
			t.Errorf("minorOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBundledExtensionsUnknownVersion(t *testing.T) {
	if got := BundledExtensions("7.4"); got != nil {
		t.Errorf("BundledExtensions(7.4) = %v, want nil", got)
	}
}

func TestBaseIniFileName(t *testing.T) {
	cases := []struct {
		input    string
		want     string
		wantWarn bool
	}{
		{"production", "php.ini-production", false},
		{"development", "php.ini-development", false},
		{"none", "", false},
		{"php.ini-production", "php.ini-production", false},
		{"php.ini-development", "php.ini-development", false},
		{"", "php.ini-production", false},       // default
		{"garbage", "php.ini-production", true}, // fallback with warning
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, warn := BaseIniFileName(tc.input)
			if got != tc.want {
				t.Errorf("BaseIniFileName(%q) = %q, want %q", tc.input, got, tc.want)
			}
			if (warn != "") != tc.wantWarn {
				t.Errorf("BaseIniFileName(%q) warn = %q, wantWarn=%v", tc.input, warn, tc.wantWarn)
			}
			if tc.wantWarn && !strings.HasPrefix(warn, "::warning::") {
				t.Errorf("BaseIniFileName(%q) warn missing ::warning:: prefix: %q", tc.input, warn)
			}
			// Full warning-text assertion for the garbage case — locks the format so
			// future changes to BaseIniFileName's warning phrasing force a deliberate
			// test update.
			if tc.input == "garbage" {
				wantWarn := "::warning::input 'ini-file=garbage' is not a recognized value; falling back to production"
				if warn != wantWarn {
					t.Errorf("BaseIniFileName(garbage) warn = %q, want %q", warn, wantWarn)
				}
			}
		})
	}
}

func TestXdebugIniFragment(t *testing.T) {
	cases := []struct {
		php  string
		want map[string]string
	}{
		{"8.4", map[string]string{"xdebug.mode": "coverage"}},
		{"8.4.5", map[string]string{"xdebug.mode": "coverage"}},
		{"8.0", map[string]string{"xdebug.mode": "coverage"}},
		{"8.9.99", map[string]string{"xdebug.mode": "coverage"}},
		{"7.4", map[string]string{"xdebug.mode": "coverage"}},
		{"7.3", map[string]string{"xdebug.mode": "coverage"}},
		{"7.2", map[string]string{"xdebug.mode": "coverage"}},
		{"7.1", nil}, // outside xdebug3_versions
		{"9.0", nil}, // outside xdebug3_versions
		{"", nil},
	}
	for _, tc := range cases {
		t.Run(tc.php, func(t *testing.T) {
			got := XdebugIniFragment(tc.php)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("XdebugIniFragment(%q) = %v, want %v", tc.php, got, tc.want)
			}
		})
	}
}

// After the OpcacheIniFragment split, DefaultIniValues returns the same 2-key
// set on every PHP version and arch. Opcache.* moved to OpcacheIniFragment.
func TestDefaultIniValues_VersionAndArchInvariant(t *testing.T) {
	want := map[string]string{
		"date.timezone": "UTC",
		"memory_limit":  "-1",
	}
	for _, arch := range []string{"x86_64", "aarch64"} {
		for _, php := range []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.4.5", "9.0"} {
			t.Run(arch+"/"+php, func(t *testing.T) {
				got := DefaultIniValues(php, arch)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("DefaultIniValues(%q, %q) = %v, want %v", php, arch, got, want)
				}
			})
		}
	}
}

func TestOpcacheIniFragmentMatchesGoldens(t *testing.T) {
	cases := []struct {
		arch   string
		golden string
	}{
		{"x86_64", "opcache_ini_fragment_x86_64.golden"},
		{"aarch64", "opcache_ini_fragment_aarch64.golden"},
	}
	for _, tc := range cases {
		t.Run(tc.arch, func(t *testing.T) {
			got := OpcacheIniFragment("8.4", tc.arch)
			want := map[string]string{}
			for _, line := range readGoldenLines(t, tc.golden) {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					t.Fatalf("bad golden line: %q", line)
				}
				want[parts[0]] = parts[1]
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("OpcacheIniFragment(8.4, %q) = %v, want %v", tc.arch, got, want)
			}
		})
	}
}

func TestOpcacheIniFragment_AppliesToAllPHP8x(t *testing.T) {
	for _, php := range []string{"8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.4.5", "8.9"} {
		t.Run(php, func(t *testing.T) {
			if got := OpcacheIniFragment(php, "x86_64"); got == nil {
				t.Errorf("OpcacheIniFragment(%q, x86_64) = nil, want non-nil", php)
			}
		})
	}
}

func TestOpcacheIniFragment_NilOutsidePHP8x(t *testing.T) {
	for _, arch := range []string{"x86_64", "aarch64"} {
		for _, php := range []string{"7.4", "9.0", ""} {
			t.Run(arch+"/"+php, func(t *testing.T) {
				if got := OpcacheIniFragment(php, arch); got != nil {
					t.Errorf("OpcacheIniFragment(%q, %q) = %v, want nil", php, arch, got)
				}
			})
		}
	}
}

func TestStockIniDefaultsMatchesGolden(t *testing.T) {
	got := StockIniDefaults()

	want := map[string]string{}
	for _, line := range readGoldenLines(t, "stock_ini_defaults.golden") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("bad golden line: %q", line)
		}
		want[parts[0]] = parts[1]
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("StockIniDefaults() = %v, want %v", got, want)
	}
}

// readGoldenLines reads testdata/<name>, strips blank lines and # comments,
// returns the remaining non-empty lines in file order.
func readGoldenLines(t *testing.T, name string) []string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	var out []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
