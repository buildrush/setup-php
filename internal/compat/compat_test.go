package compat

import (
	"bufio"
	"os"
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

	f, err := os.Open("testdata/default_ini_values.golden")
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()

	want := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
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
