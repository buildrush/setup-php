package build

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

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

// PushAll promotes every keyed bundle in source to dest. Source MUST be
// an oci-layout populated by `phpup build`, where each manifest carries
// the `io.buildrush.bundle.key` annotation — that's what lets us recover
// the canonical publication tag (e.g. "6.3.0-8.5-nts-linux-x86_64" for
// `ext:redis:6.3.0:8.5:linux:x86_64:nts`) on the destination.
//
// Manifests without a bundle-key annotation are skipped silently by
// ListKeyed; the `phpup build` path always sets it. Forward push paths
// only see build-produced layouts, so this exposure is bounded.
//
// The destination manifest is annotated with BundleName/BundleKey/SpecHash
// so consumers can resolve via canonical tag (lockfile-update) AND via
// LookupBySpec (build-cache probe).
func PushAll(ctx context.Context, sourceURI, destURI string) error {
	src, err := registry.Open(sourceURI)
	if err != nil {
		return fmt.Errorf("phpup push: open source %s: %w", sourceURI, err)
	}
	dst, err := registry.Open(destURI)
	if err != nil {
		return fmt.Errorf("phpup push: open dest %s: %w", destURI, err)
	}
	// Source MUST expose ListKeyed so we can recover the canonical tag
	// from each manifest's bundle-key annotation. Only the layout backend
	// implements it natively; a remote source would need a custom API to
	// enumerate annotated manifests.
	lister, ok := src.(interface {
		ListKeyed(ctx context.Context) ([]registry.KeyedRef, error)
	})
	if !ok {
		return fmt.Errorf("phpup push: source %q doesn't support ListKeyed (use oci-layout:<path>)", sourceURI)
	}
	refs, err := lister.ListKeyed(ctx)
	if err != nil {
		return fmt.Errorf("phpup push: list source: %w", err)
	}
	fmt.Printf("phpup push: promoting %d manifests from %s to %s\n", len(refs), sourceURI, destURI)
	for _, kref := range refs {
		if err := promoteOne(ctx, src, dst, kref); err != nil {
			return fmt.Errorf("phpup push: promote %s: %w", kref.Key, err)
		}
	}
	return nil
}

// promoteOne fetches a single source manifest's bundle + meta and pushes
// the payload to the destination under the canonical tag derived from
// the bundle key. The destination manifest carries the propagated
// BundleKey + SpecHash annotations so post-push lockfile-update and
// LookupBySpec both resolve correctly.
func promoteOne(ctx context.Context, src, dst registry.Store, kref registry.KeyedRef) error {
	imageName, tag, err := canonicalRefFromBundleKey(kref.Key)
	if err != nil {
		return err
	}
	rc, meta, err := src.Fetch(ctx, registry.Ref{Name: kref.Name, Digest: kref.Digest})
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = rc.Close() }()
	dstRef := registry.Ref{Name: imageName, Tag: tag}
	ann := registry.Annotations{
		BundleName: imageName,
		BundleKey:  kref.Key,
		SpecHash:   kref.SpecHash,
	}
	return dst.Push(ctx, dstRef, rc, meta, ann)
}

// canonicalRefFromBundleKey parses a bundle key produced by
// internal/lockfile.{PHPBundleKey,ExtBundleKey} and returns the
// (image-name, tag) pair that mirrors what `phpup lockfile-update`
// queries against ghcr.io. The two paths must agree for `phpup push` →
// `lockfile-update` to round-trip.
//
//	php:<ver>:<os>:<arch>:<ts>
//	  → ("php-core",       "<ver>-<os>-<arch>-<ts>")
//	ext:<name>:<ver>:<phpver>:<os>:<arch>:<ts>
//	  → ("php-ext-<name>", "<ver>-<phpver>-<ts>-<os>-<arch>")
//
// Tool keys aren't supported yet — `phpup build` doesn't produce them
// and `lockfile-update` doesn't query them. Returns an explicit error
// rather than guessing a layout.
func canonicalRefFromBundleKey(key string) (imageName, tag string, err error) {
	parts := strings.Split(key, ":")
	switch {
	case len(parts) == 5 && parts[0] == "php":
		ver, osName, arch, ts := parts[1], parts[2], parts[3], parts[4]
		return "php-core", fmt.Sprintf("%s-%s-%s-%s", ver, osName, arch, ts), nil
	case len(parts) == 7 && parts[0] == "ext":
		name, ver, phpver, osName, arch, ts := parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]
		return "php-ext-" + name, fmt.Sprintf("%s-%s-%s-%s-%s", ver, phpver, ts, osName, arch), nil
	default:
		return "", "", fmt.Errorf("canonicalRefFromBundleKey: unrecognized bundle key shape %q", key)
	}
}
