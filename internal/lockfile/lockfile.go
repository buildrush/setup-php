package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BundleKey is a canonical key like "php:8.4.6:linux:x86_64:nts".
type BundleKey = string

// Digest is a content-addressed SHA-256 digest like "sha256:abc123".
type Digest = string

// Entry pairs a bundle digest with the spec_hash of the inputs that produced
// it. Empty SpecHash means the entry predates the spec_hash era and is
// grandfathered (treated as fresh until something forces a rebuild).
type Entry struct {
	Digest   string `json:"digest"`
	SpecHash string `json:"spec_hash,omitempty"`
}

// Lockfile represents the bundles.lock JSON structure.
type Lockfile struct {
	SchemaVersion int                 `json:"schema_version"`
	GeneratedAt   time.Time           `json:"generated_at"`
	Bundles       map[BundleKey]Entry `json:"bundles"`
}

const currentSchemaVersion = 2

// rawV1 accepts the legacy string-valued map for migration in Parse.
type rawV1 struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Bundles       map[BundleKey]any `json:"bundles"`
}

// Parse parses a lockfile from JSON bytes. Accepts schema v1 (values are bare
// digest strings) and v2 (values are Entry objects). v1 entries are migrated
// in memory to Entry{Digest: <string>, SpecHash: ""}.
func Parse(data []byte) (*Lockfile, error) {
	var probe rawV1
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse lockfile: %w", err)
	}
	if probe.SchemaVersion < 1 || probe.SchemaVersion > currentSchemaVersion {
		return nil, fmt.Errorf("unsupported lockfile schema version %d (expected 1 or %d)",
			probe.SchemaVersion, currentSchemaVersion)
	}
	lf := &Lockfile{
		SchemaVersion: currentSchemaVersion,
		GeneratedAt:   probe.GeneratedAt,
		Bundles:       make(map[BundleKey]Entry, len(probe.Bundles)),
	}
	for k, v := range probe.Bundles {
		switch val := v.(type) {
		case string:
			// v1 bare digest
			lf.Bundles[k] = Entry{Digest: val}
		case map[string]any:
			// v2 struct form
			digest, _ := val["digest"].(string)
			specHash, _ := val["spec_hash"].(string)
			if digest == "" {
				return nil, fmt.Errorf("parse lockfile: entry %q missing digest", k)
			}
			lf.Bundles[k] = Entry{Digest: digest, SpecHash: specHash}
		default:
			return nil, fmt.Errorf("parse lockfile: entry %q has unexpected type %T", k, v)
		}
	}
	return lf, nil
}

// ParseFile reads and parses a lockfile from disk.
func ParseFile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read lockfile %s: %w", path, err)
	}
	return Parse(data)
}

// Write serializes the lockfile to disk as indented JSON.
func (lf *Lockfile) Write(path string) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

// Lookup returns the digest for a bundle key, or false if not found.
// Kept for backward compatibility with existing callers that only need the digest.
func (lf *Lockfile) Lookup(key BundleKey) (Digest, bool) {
	e, ok := lf.Bundles[key]
	return e.Digest, ok
}

// LookupEntry returns the full Entry for a bundle key, or false if not found.
func (lf *Lockfile) LookupEntry(key BundleKey) (Entry, bool) {
	e, ok := lf.Bundles[key]
	return e, ok
}

// Set adds or updates a bundle entry with digest only; spec_hash becomes empty.
// Prefer SetEntry for callers that know the spec_hash.
func (lf *Lockfile) Set(key BundleKey, digest Digest) {
	lf.Bundles[key] = Entry{Digest: digest}
}

// SetEntry adds or updates a full bundle entry.
func (lf *Lockfile) SetEntry(key BundleKey, entry Entry) {
	lf.Bundles[key] = entry
}

// PHPBundleKey constructs a canonical key for a PHP core bundle.
func PHPBundleKey(version, osName, arch, ts string) BundleKey {
	return fmt.Sprintf("php:%s:%s:%s:%s", version, osName, arch, ts)
}

// ExtBundleKey constructs a canonical key for an extension bundle.
func ExtBundleKey(name, version, phpVersion, osName, arch, ts string) BundleKey {
	return fmt.Sprintf("ext:%s:%s:%s:%s:%s:%s", name, version, phpVersion, osName, arch, ts)
}

// ToolBundleKey constructs a canonical key for a tool bundle.
func ToolBundleKey(name, version string) BundleKey {
	return fmt.Sprintf("tool:%s:%s:any:any", name, version)
}
