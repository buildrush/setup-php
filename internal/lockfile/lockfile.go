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

// Lockfile represents the bundles.lock JSON structure.
type Lockfile struct {
	SchemaVersion int                  `json:"schema_version"`
	GeneratedAt   time.Time            `json:"generated_at"`
	Bundles       map[BundleKey]Digest `json:"bundles"`
}

const currentSchemaVersion = 1

// Parse parses a lockfile from JSON bytes.
func Parse(data []byte) (*Lockfile, error) {
	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lockfile: %w", err)
	}
	if lf.SchemaVersion != currentSchemaVersion {
		return nil, fmt.Errorf("unsupported lockfile schema version %d (expected %d)", lf.SchemaVersion, currentSchemaVersion)
	}
	if lf.Bundles == nil {
		lf.Bundles = make(map[BundleKey]Digest)
	}
	return &lf, nil
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
func (lf *Lockfile) Lookup(key BundleKey) (Digest, bool) {
	d, ok := lf.Bundles[key]
	return d, ok
}

// Set adds or updates a bundle entry.
func (lf *Lockfile) Set(key BundleKey, digest Digest) {
	lf.Bundles[key] = digest
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
