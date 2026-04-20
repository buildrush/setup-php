package version

import "testing"

func TestMinBundleSchema_KnownKinds(t *testing.T) {
	tests := []struct {
		kind string
		want int
	}{
		{"php-core", 2},
		{"php-ext", 1},
	}
	for _, tt := range tests {
		if got := MinBundleSchema(tt.kind); got != tt.want {
			t.Errorf("MinBundleSchema(%q) = %d, want %d", tt.kind, got, tt.want)
		}
	}
}

func TestMinBundleSchema_UnknownKind_Zero(t *testing.T) {
	// Unknown kind means compose has no constraint for this bundle and
	// should rely on its own validation. Returning 0 (no minimum) keeps
	// the compose assertion a strict >= comparison.
	if got := MinBundleSchema("php-tool"); got != 0 {
		t.Errorf("MinBundleSchema(\"php-tool\") = %d, want 0 (unknown kind)", got)
	}
}
