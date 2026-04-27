// Package layoutlockfile synthesises a phpup lockfile JSON from the annotated
// manifests in a local OCI image layout. The output is consumed by smoke-test
// runners that need to resolve bundles against a local layout whose digests
// differ from the embedded bundles.lock.
package layoutlockfile

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/buildrush/setup-php/internal/registry"
)

// lister is a sub-interface of registry.Store that exposes ListKeyed.
// The layout backend implements it; remote backends do not.
type lister interface {
	ListKeyed(ctx context.Context) ([]registry.KeyedRef, error)
}

// WriteSynthesized walks the OCI layout at layoutURI, reads each manifest's
// io.buildrush.bundle.key annotation, and writes a phpup lockfile JSON to
// outPath. Parent directories of outPath are created as needed.
//
// layoutURI must be in the form accepted by registry.Open, e.g.
// "oci-layout:/abs/path".  outPath is the full filesystem path of the file to
// create or overwrite.
//
// Output JSON shape:
//
//	{
//	  "schema_version": 2,
//	  "generated_at":   "<RFC3339 UTC>",
//	  "bundles": {
//	    "<key>": {"digest": "sha256:...", "spec_hash": "sha256:..."}
//	  }
//	}
//
// The spec_hash field is omitted for bundles whose annotation is empty.
func WriteSynthesized(layoutURI, outPath string) error {
	store, err := registry.Open(layoutURI)
	if err != nil {
		return fmt.Errorf("open layout: %w", err)
	}
	ls, ok := store.(lister)
	if !ok {
		return errors.New("layout store does not implement ListKeyed")
	}
	refs, err := ls.ListKeyed(context.Background())
	if err != nil {
		return fmt.Errorf("list keyed refs: %w", err)
	}

	bundles := make(map[string]map[string]string, len(refs))
	for _, r := range refs {
		entry := map[string]string{"digest": r.Digest}
		if r.SpecHash != "" {
			entry["spec_hash"] = r.SpecHash
		}
		bundles[r.Key] = entry
	}

	doc := map[string]any{
		"schema_version": 2,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"bundles":        bundles,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}
	return nil
}

// MainCLI is a flag.NewFlagSet-based entry point suitable for wiring into a
// CLI subcommand (e.g. "phpup write-layout-lockfile"). It accepts:
//
//	--layout <uri>   OCI layout URI, passed verbatim to registry.Open.
//	--out    <path>  Output file path.
//
// Both flags are required; MainCLI returns an error if either is missing.
func MainCLI(args []string) error {
	fs := flag.NewFlagSet("write-layout-lockfile", flag.ContinueOnError)
	layout := fs.String("layout", "", "OCI layout URI (required)")
	out := fs.String("out", "", "output lockfile path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *layout == "" {
		return errors.New("--layout is required")
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	return WriteSynthesized(*layout, *out)
}
