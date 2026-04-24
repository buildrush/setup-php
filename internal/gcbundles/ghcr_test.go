package gcbundles

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fakeRunner records calls and returns canned responses keyed by the joined
// arg string. Unknown commands return an error.
type fakeRunner struct {
	responses map[string]string
	calls     []string
}

func (f *fakeRunner) Run(args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if resp, ok := f.responses[key]; ok {
		return []byte(resp), nil
	}
	return nil, &stubError{key: key}
}

type stubError struct{ key string }

func (e *stubError) Error() string { return "no canned response for: " + e.key }

func TestListPackageVersions_ParsesGHCRResponse(t *testing.T) {
	manifests := []GHCRVersion{
		{ID: 111, Name: "sha256:aaa", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 222, Name: "sha256:bbb", CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}
	payload, _ := json.Marshal(manifests)

	runner := &fakeRunner{responses: map[string]string{
		"api --paginate /orgs/buildrush/packages/container/php-core/versions?per_page=100": string(payload),
	}}

	got, err := ListPackageVersions(runner, "buildrush", "php-core")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != 111 || got[1].ID != 222 {
		t.Errorf("IDs = [%d, %d], want [111, 222]", got[0].ID, got[1].ID)
	}
	if !got[0].CreatedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("first CreatedAt = %v, want 2026-01-01", got[0].CreatedAt)
	}
}
