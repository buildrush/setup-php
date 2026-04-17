package compat

import (
	"fmt"
	"os"
	"path/filepath"
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
	got := DefaultIniValues("8.4")

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
			want := readGoldenLines(t, fmt.Sprintf("bundled_extensions_%s.golden", v))
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

func TestBundledExtensionsUnknownVersion(t *testing.T) {
	if got := BundledExtensions("7.4"); got != nil {
		t.Errorf("BundledExtensions(7.4) = %v, want nil", got)
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
