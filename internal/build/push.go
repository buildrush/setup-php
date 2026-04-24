package build

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/buildrush/setup-php/internal/registry"
)

// PushMain is the entry point for `phpup push …`. Parses the --from /
// --to flags and delegates to PushAll. Returns a non-nil error safe to
// pass straight to log.Fatalf.
func PushMain(args []string) error {
	fs := flag.NewFlagSet("phpup push", flag.ContinueOnError)
	from := fs.String("from", "", "Source registry (oci-layout:<path>); REQUIRED")
	to := fs.String("to", "", "Destination registry (e.g. ghcr.io/<owner>); REQUIRED")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		return errors.New("phpup push: --from and --to are required")
	}
	return PushAll(context.Background(), *from, *to)
}

// PushAll promotes every bundle in source to dest. Source MUST be an
// oci-layout (remote->remote copy isn't supported in PR 4; use oras for
// that case) — a remote source is rejected up front with an error
// mentioning the supported URI form.
//
// The walk reads the source layout's index, and for each manifest
// re-pushes the layer bytes + meta to the destination via Store.Push.
// Tag derivation is currently lossy: the source manifest's original
// publication tag isn't recoverable from Fetch, so deriveTagForPromotion
// falls back to a digest-prefix tag like "sha256-abc123…". PR 6 will
// carry the original tag via a manifest annotation.
//
// The bundle-name annotation is preserved (via Annotations.BundleName)
// so the destination still resolves through LookupBySpec / Has for
// downstream consumers. The source-side spec-hash is not propagated —
// Fetch doesn't surface manifest annotations, so the promoted manifest
// ends up annotated only by what ref.Name implies. When/if LookupBySpec
// on a promoted remote becomes required, the Store interface will need
// an annotations-accessor.
func PushAll(ctx context.Context, sourceURI, destURI string) error {
	src, err := registry.Open(sourceURI)
	if err != nil {
		return fmt.Errorf("phpup push: open source %s: %w", sourceURI, err)
	}
	dst, err := registry.Open(destURI)
	if err != nil {
		return fmt.Errorf("phpup push: open dest %s: %w", destURI, err)
	}
	// Source MUST expose an index walk. Only the layout backend does so
	// natively in PR 4; a remote source would need a custom API to
	// enumerate tagged manifests (deferred to PR 6 consolidation).
	lister, ok := src.(interface {
		List(ctx context.Context) ([]registry.Ref, error)
	})
	if !ok {
		return fmt.Errorf("phpup push: source %q doesn't support index walk (use oci-layout:<path>)", sourceURI)
	}
	refs, err := lister.List(ctx)
	if err != nil {
		return fmt.Errorf("phpup push: list source: %w", err)
	}
	fmt.Printf("phpup push: promoting %d manifests from %s to %s\n", len(refs), sourceURI, destURI)
	for _, ref := range refs {
		if err := promoteOne(ctx, src, dst, ref); err != nil {
			return fmt.Errorf("phpup push: promote %s: %w", ref, err)
		}
	}
	return nil
}

// promoteOne fetches a single source manifest's bundle + meta and
// pushes the payload to the destination under a derived tag. Close
// failures on the source reader are swallowed — the push has already
// succeeded at that point and the caller's next action is typically to
// print a success line and exit.
func promoteOne(ctx context.Context, src, dst registry.Store, ref registry.Ref) error {
	rc, meta, err := src.Fetch(ctx, ref)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = rc.Close() }()
	tag := deriveTagForPromotion(ref)
	if tag == "" {
		return fmt.Errorf("couldn't derive tag for %s (ref has no digest)", ref)
	}
	if ref.Name == "" {
		return fmt.Errorf("couldn't derive name for %s (ref has no bundle-name annotation)", ref)
	}
	dstRef := registry.Ref{Name: ref.Name, Tag: tag}
	ann := registry.Annotations{BundleName: ref.Name}
	return dst.Push(ctx, dstRef, rc, meta, ann)
}

// deriveTagForPromotion produces a tag string for a manifest based on
// its digest. The canonical publication tag shape
// (e.g. "<php-ver>-<os>-<arch>-<ts>" for php-core) isn't recoverable
// from the Fetch return — go-containerregistry's layout reader doesn't
// expose per-manifest tags on the index, and the digest alone doesn't
// encode them. For PR 4 we fall back to "sha256-<first12hex>" so the
// destination registry has SOMETHING to address, at the cost of losing
// human-readable tag semantics. PR 6's CLI consolidation will thread
// the original tag via the source manifest's annotation so promotions
// stay addressable under their published names.
func deriveTagForPromotion(ref registry.Ref) string {
	if ref.Digest == "" {
		return ""
	}
	// "sha256:" prefix is 7 characters; the hex payload that follows is
	// 64 characters for sha256. Take 12 hex chars after the prefix —
	// enough to avoid collisions in any realistic layout, short enough
	// to stay within the OCI tag length limit (128 chars).
	d := ref.Digest
	if len(d) > 19 {
		return "sha256-" + d[7:19]
	}
	// Pathological fallback — ref.Digest looks like "sha256:<short>"
	// which shouldn't happen in practice but we keep the behavior
	// defined rather than panicking.
	return "sha256-" + d[7:]
}
