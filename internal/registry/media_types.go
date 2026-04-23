package registry

import "strings"

// Media types and artifact types for bundles published to a remote registry.
//
// Keeping these byte-identical to what the existing `oras push` commands in
// .github/workflows/build-php-core.yml and .github/workflows/build-extension.yml
// emit is a hard requirement: a remoteStore-pushed artifact must be
// indistinguishable from an oras-pushed one, because downstream tooling
// (cosign signing, OCI clients, human operators running `oras discover`)
// keys off these strings.
const (
	// mediaTypePhpCoreArtifact is the oras --artifact-type for php-core
	// bundles. Matches build-php-core.yml.
	mediaTypePhpCoreArtifact = "application/vnd.buildrush.php-core.v1"
	// mediaTypePhpExtArtifact is the oras --artifact-type for php-ext-*
	// bundles. Matches build-extension.yml.
	mediaTypePhpExtArtifact = "application/vnd.buildrush.php-ext.v1"
	// mediaTypeBundleLayer is the OCI media type for the bundle tar.zst
	// blob. Matches both build-*.yml files.
	mediaTypeBundleLayer = "application/vnd.oci.image.layer.v1.tar+zstd"
	// mediaTypeMetaSidecar is the media type for the meta.json sidecar.
	// Matches both build-*.yml files.
	mediaTypeMetaSidecar = "application/vnd.buildrush.meta.v1+json"
	// annotationArtifactType is the OCI manifest annotation key `oras push`
	// writes when invoked with --artifact-type. Replayed here so remote
	// pushes carry the same annotation shape as the CI path.
	annotationArtifactType = "org.opencontainers.artifact.type"
)

// artifactTypeForBundle maps a bundle Name to the artifact-type annotation
// value that `oras push --artifact-type <X>` would set.
//
// The split mirrors the workflow files: php-ext-<anything> → phpExt, every
// other name → phpCore. The default keeps php-core + any future php-tool-*
// bundles on the phpCore artifact-type, matching pre-merge behavior in the
// existing CI path.
func artifactTypeForBundle(name string) string {
	if strings.HasPrefix(name, "php-ext-") {
		return mediaTypePhpExtArtifact
	}
	return mediaTypePhpCoreArtifact
}
