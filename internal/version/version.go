package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// MinBundleSchema returns the minimum sidecar schema_version the runtime
// requires for the given bundle kind. 0 means "no minimum" (the kind is
// unknown to this build of the runtime — compose will not gate on it).
//
// Versions are compared as integers; bump here in the same slice that
// introduces a runtime assertion on new sidecar content. Also record the
// bump in docs/bundle-schema-changelog.md.
func MinBundleSchema(kind string) int {
	switch kind {
	case "php-core":
		return 3
	case "php-ext":
		return 3
	default:
		return 0
	}
}
