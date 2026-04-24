package registry

// annotationSpecHash is the OCI-annotation key where the build spec-hash
// lives on the manifest. annotationBundleName is defined in layout.go.
const (
	annotationSpecHash  = "io.buildrush.bundle.spec-hash"
	annotationBundleKey = "io.buildrush.bundle.key"
)

// Annotations is a well-known, backend-agnostic set of manifest annotations
// that callers can request on Push and query on LookupBySpec. Keys are OCI
// annotation strings under the io.buildrush namespace so external OCI
// inspection tools (oras discover, skopeo inspect) see them too.
type Annotations struct {
	// BundleName is the logical bundle kind+name (e.g. "php-core",
	// "php-ext-redis"). Mirrors the annotationBundleName key used by PR 1.
	BundleName string

	// SpecHash is the deterministic hash of the inputs that produced the
	// bundle (builder scripts + catalog entry + os/arch/php/ts). Used by
	// the build subcommand's cache probe to skip redundant rebuilds.
	SpecHash string

	// BundleKey is the canonical lockfile key, e.g.
	// "php:8.4:linux:x86_64:nts" or "ext:redis:6.2.0:8.4-nts:linux:x86_64".
	// Set by phpup build php|ext; read by phpup test's lockfile-override
	// synthesis so test containers can resolve bundles against a local
	// layout whose digests differ from the embedded bundles.lock.
	BundleKey string
}

// asMap returns the OCI annotation map for the Annotations value.
// Empty fields are omitted so we don't write empty-string annotations.
// The returned map is always non-nil and writable — callers can mutate
// it in place (e.g. the layout backend's back-compat fallback writes
// BundleName from ref.Name when the Annotations value is zero).
func (a Annotations) asMap() map[string]string {
	m := map[string]string{}
	if a.BundleName != "" {
		m[annotationBundleName] = a.BundleName
	}
	if a.SpecHash != "" {
		m[annotationSpecHash] = a.SpecHash
	}
	if a.BundleKey != "" {
		m[annotationBundleKey] = a.BundleKey
	}
	return m
}
