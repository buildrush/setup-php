package compat

import "testing"

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
