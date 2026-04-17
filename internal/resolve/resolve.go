package resolve

import (
	"fmt"
	"strings"

	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/lockfile"
	"github.com/buildrush/setup-php/internal/plan"
)

type ResolvedBundle struct {
	Key     string
	Digest  string
	Name    string
	Version string
	Kind    string // "php", "ext", "tool"
}

type Resolution struct {
	PHPCore    ResolvedBundle
	Extensions []ResolvedBundle
	Warnings   []string
}

func Resolve(p *plan.Plan, lf *lockfile.Lockfile, cat *catalog.Catalog) (*Resolution, error) {
	res := &Resolution{}

	phpKey := lockfile.PHPBundleKey(p.PHPVersion, p.OS, p.Arch, p.ThreadSafety)
	phpDigest, ok := lf.Lookup(phpKey)
	if !ok && p.ThreadSafety == "zts" {
		// Fall back to NTS.
		ntsKey := lockfile.PHPBundleKey(p.PHPVersion, p.OS, p.Arch, "nts")
		if ntsDigest, ok2 := lf.Lookup(ntsKey); ok2 {
			if p.FailFast {
				return nil, fmt.Errorf("PHP %s ZTS for %s/%s not in lockfile (fail-fast)", p.PHPVersion, p.OS, p.Arch)
			}
			phpKey = ntsKey
			phpDigest = ntsDigest
			ok = true
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("::warning::PHP %s ZTS not available for %s/%s; falling back to NTS", p.PHPVersion, p.OS, p.Arch))
		}
	}
	if !ok {
		return nil, fmt.Errorf("PHP %s for %s/%s/%s not found in lockfile", p.PHPVersion, p.OS, p.Arch, p.ThreadSafety)
	}
	res.PHPCore = ResolvedBundle{
		Key:     phpKey,
		Digest:  phpDigest,
		Name:    "php",
		Version: p.PHPVersion,
		Kind:    "php",
	}

	for _, extName := range p.Extensions {
		if cat.IsBundled(extName) {
			continue
		}

		extSpec, ok := cat.Extensions[extName]
		if !ok {
			return nil, fmt.Errorf("extension %q not found in catalog", extName)
		}

		if extSpec.Kind == catalog.ExtensionKindBundled {
			continue
		}

		phpMinor := extractMinor(p.PHPVersion)

		var extVersion string
		var extKey string
		var extDigest string

		if len(extSpec.Versions) > 0 {
			// Use the first known version and build the key directly.
			extVersion = extSpec.Versions[0]
			extKey = lockfile.ExtBundleKey(extName, extVersion, phpMinor, p.OS, p.Arch, p.ThreadSafety)
			extDigest, ok = lf.Lookup(extKey)
			if !ok {
				return nil, fmt.Errorf("extension %s %s for PHP %s %s/%s/%s not found in lockfile", extName, extVersion, phpMinor, p.OS, p.Arch, p.ThreadSafety)
			}
		} else {
			// No version pinned in catalog — scan the lockfile for a matching key.
			prefix := fmt.Sprintf("ext:%s:", extName)
			suffix := fmt.Sprintf(":%s:%s:%s:%s", phpMinor, p.OS, p.Arch, p.ThreadSafety)
			found := false
			for k, d := range lf.Bundles {
				if !strings.HasPrefix(k, prefix) || !strings.HasSuffix(k, suffix) {
					continue
				}
				extKey = k
				extDigest = d
				// Extract version from key: ext:<name>:<version>:<phpMinor>:...
				inner := k[len(prefix):]
				extVersion = strings.SplitN(inner, ":", 2)[0]
				found = true
				break
			}
			if !found {
				return nil, fmt.Errorf("extension %s for PHP %s %s/%s/%s not found in lockfile", extName, phpMinor, p.OS, p.Arch, p.ThreadSafety)
			}
		}

		res.Extensions = append(res.Extensions, ResolvedBundle{
			Key:     extKey,
			Digest:  extDigest,
			Name:    extName,
			Version: extVersion,
			Kind:    "ext",
		})
	}

	if len(p.Tools) > 0 {
		msg := fmt.Sprintf("tools input is not yet supported (requested: %s); will be ignored", strings.Join(p.Tools, ","))
		if p.FailFast {
			return nil, fmt.Errorf("%s (fail-fast)", msg)
		}
		res.Warnings = append(res.Warnings, "::warning::"+msg)
	}

	return res, nil
}

func extractMinor(version string) string {
	parts := 0
	for i, c := range version {
		if c == '.' {
			parts++
			if parts == 2 {
				return version[:i]
			}
		}
	}
	return version
}
