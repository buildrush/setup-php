package hermeticaudit

import (
	"reflect"
	"testing"
)

func TestParseLddOutputNotFound(t *testing.T) {
	input := `
	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007f...)
	libicui18n.so.70 => not found
	libstdc++.so.6 => /lib/x86_64-linux-gnu/libstdc++.so.6 (0x00007f...)
	libmagickwand.so.5 => not found
`
	missing := parseLddNotFound(input)
	want := []string{"libicui18n.so.70", "libmagickwand.so.5"}
	if !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}

func TestClassifyMissing(t *testing.T) {
	declared := []string{"libicui18n.so.*", "libicuuc.so.*"}
	cases := []struct {
		name            string
		missing         []string
		wantUnexplained []string
		wantCaptureBugs []string
	}{
		{"all matched glob → capture bug", []string{"libicui18n.so.70"}, nil, []string{"libicui18n.so.70"}},
		{"unmatched → unexplained", []string{"libmagickwand.so.5"}, []string{"libmagickwand.so.5"}, nil},
		{"mixed", []string{"libicui18n.so.70", "libpq.so.5"}, []string{"libpq.so.5"}, []string{"libicui18n.so.70"}},
		{"none missing", nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unexp, capBugs := classifyMissing(tc.missing, declared)
			if !reflect.DeepEqual(unexp, tc.wantUnexplained) {
				t.Errorf("unexplained = %v, want %v", unexp, tc.wantUnexplained)
			}
			if !reflect.DeepEqual(capBugs, tc.wantCaptureBugs) {
				t.Errorf("captureBugs = %v, want %v", capBugs, tc.wantCaptureBugs)
			}
		})
	}
}

func TestSuggestGlob(t *testing.T) {
	cases := []struct {
		missing string
		want    string
	}{
		{"libpq.so.5", "libpq.so.*"},
		{"libMagickWand-6.Q16.so.6", "libMagickWand-6.Q16.so.*"},
		{"libc.so", "libc.so.*"},
	}
	for _, tc := range cases {
		t.Run(tc.missing, func(t *testing.T) {
			got := suggestGlob(tc.missing)
			if got != tc.want {
				t.Errorf("suggestGlob(%q) = %q, want %q", tc.missing, got, tc.want)
			}
		})
	}
}
