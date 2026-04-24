package gcbundles

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// GHCRVersion is the subset of a GHCR package version response we care about.
type GHCRVersion struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"` // e.g. "sha256:abc..."
	CreatedAt time.Time `json:"created_at"`
}

// Runner abstracts the gh CLI invocation so tests can inject canned responses.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

// ghRunner is the production Runner that execs `gh`.
type ghRunner struct{}

func (ghRunner) Run(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %v: %w\n%s", args, err, out)
	}
	return out, nil
}

// NewGHRunner returns a production Runner that invokes the gh CLI.
func NewGHRunner() Runner {
	return ghRunner{}
}

// ListPackageVersions returns every version manifest of a GHCR package.
func ListPackageVersions(r Runner, org, pkg string) ([]GHCRVersion, error) {
	path := fmt.Sprintf("/orgs/%s/packages/container/%s/versions?per_page=100", org, pkg)
	out, err := r.Run("api", "--paginate", path)
	if err != nil {
		return nil, err
	}
	var versions []GHCRVersion
	if err := json.Unmarshal(out, &versions); err != nil {
		return nil, fmt.Errorf("parse versions for %s: %w", pkg, err)
	}
	return versions, nil
}

// DeletePackageVersion removes a single version. Caller must be in --confirm
// mode before invoking.
func DeletePackageVersion(r Runner, org, pkg string, id int64) error {
	path := fmt.Sprintf("/orgs/%s/packages/container/%s/versions/%d", org, pkg, id)
	_, err := r.Run("api", "--method", "DELETE", path)
	return err
}
