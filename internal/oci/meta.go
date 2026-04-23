package oci

import (
	"encoding/json"
	"fmt"
)

// parseMetaJSON decodes an OCI sidecar meta.json payload into Metadata.
// Missing schema_version defaults to 1 so pre-slice bundles referenced by
// released lockfiles remain loadable.
//
// The registry package owns equivalent decoding inside Store.Fetch; this
// helper survives here to keep the oci facade's legacy test surface intact
// until the facade itself is deleted (scheduled for end of PR 3 of the
// local+CI unification rollout).
func parseMetaJSON(data []byte) (Metadata, error) {
	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return Metadata{}, fmt.Errorf("parse meta.json: %w", err)
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	return m, nil
}
