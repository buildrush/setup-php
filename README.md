# buildrush/setup-php

Fast, reproducible PHP setup for GitHub Actions using prebuilt bundles.

> **Status:** Alpha. Linux x86_64 only. PHP 8.4.

## Quick Start

```yaml
- uses: buildrush/setup-php@v0
  with:
    php-version: '8.4'
    extensions: redis
    ini-values: memory_limit=256M
```

## Why?

`buildrush/setup-php` ships prebuilt, content-addressed OCI bundles instead of compiling PHP at CI time. Bundles are built once, signed with Sigstore, and pushed to GHCR. At setup time the action pulls the right bundle and extracts it — no `apt-get`, no compilation, no flaky mirrors.

This means setup typically completes in single-digit seconds, and every run gets a byte-identical PHP environment.

## Inputs

| Input              | Description                                                                                                    | Default      |
| ------------------ | -------------------------------------------------------------------------------------------------------------- | ------------ |
| `php-version`      | PHP version to install                                                                                         | `8.4`        |
| `phpts`            | Thread safety (`nts` or `zts`). Only `nts` bundles are published today; see compat section below.              | `nts`        |
| `extensions`       | Comma-separated extensions (supports `:ext` to exclude and `none` to reset).                                   | —            |
| `ini-values`       | Comma-separated ini settings (`key=value`).                                                                    | —            |
| `ini-file`         | Base ini template (`production` or `development`). Only `production` is currently applied; see compat section. | `production` |
| `coverage`         | Coverage driver (`xdebug`, `pcov`, `none`).                                                                    | `none`       |
| `tools`            | Comma-separated tools to install.                                                                              | —            |
| `update`           | Accepted for v2 parse compatibility; no-op under prebuilt bundles. See compat section.                         | `false`      |
| `fail-fast`        | Promote soft fallbacks (e.g. ZTS not available) to hard errors.                                                | `false`      |
| `php-version-file` | File containing PHP version.                                                                                   | —            |

## Outputs

| Output        | Description                          |
| ------------- | ------------------------------------ |
| `php-version` | Resolved PHP version (e.g., `8.4.6`) |

## Supported Matrix

| OS                   | Arch   | PHP | Thread Safety |
| -------------------- | ------ | --- | ------------- |
| Linux (Ubuntu 24.04) | x86_64 | 8.4 | NTS           |

### Bundled Extensions

These ship inside the PHP core bundle — no extra download required:

bcmath, calendar, ctype, curl, dom, exif, filter, ftp, gd, hash, iconv, intl, json, mbstring, opcache, openssl, pdo, pdo_mysql, pdo_pgsql, pdo_sqlite, pgsql, readline, session, simplexml, soap, sockets, sodium, sqlite3, tokenizer, xml, xmlreader, xmlwriter, zip, zlib

### Separately Installable Extensions

| Extension | Version |
| --------- | ------- |
| redis     | 6.2.0   |

## Compatibility with `shivammathur/setup-php@v2`

`buildrush/setup-php` is designed as a drop-in replacement for `shivammathur/setup-php@v2`. Existing workflows can migrate by changing only the `uses:` line.

Every input declared by v2 is declared here: `php-version`, `php-version-file`, `extensions`, `ini-file`, `ini-values`, `coverage`, `tools`, plus the env-var-driven `phpts`, `update`, `fail-fast`. Inputs we cannot implement given our prebuilt-bundle architecture (e.g. `update`) are accepted for parse compatibility and emit a `::warning::` line when set to a non-default value, so your workflow keeps running.

Defaults match v2 where they are observable: `date.timezone=UTC` and `memory_limit=-1` are applied unless you override them in `ini-values`. The per-PHP-version compiled-in extension baseline is audited against the `ondrej/php` PPA that v2 relies on; see [docs/compat-matrix.md](docs/compat-matrix.md) for the current delta.

Extension list syntax works the same way:

```yaml
extensions: redis, :opcache        # include redis, exclude opcache
extensions: none, redis, curl      # reset, then only redis + curl
```

Details, deliberate deviations, and deferred behavioral quirks are catalogued in [docs/compat-matrix.md](docs/compat-matrix.md).

## How we verify v2 compatibility

Every pull request and push to `main` triggers the
[`compat-harness` workflow](.github/workflows/compat-harness.yml), which runs
a fixture matrix through both `buildrush/setup-php` and
`shivammathur/setup-php@v2` (pinned by SHA in
[`docs/compat-matrix.md`](docs/compat-matrix.md)) and diffs the resulting PHP
environments. Any deviation that is not listed in the allowlist block of
`docs/compat-matrix.md` fails the check. See the spec
[`docs/superpowers/specs/2026-04-20-compat-harness-design.md`](docs/superpowers/specs/2026-04-20-compat-harness-design.md)
for the full design.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

### How PR CI handles bundle changes

When a PR modifies `catalog/**` or `builders/**`, the build pipeline self-publishes: new bundles are pushed to GHCR and `bundles.lock` is committed **directly to the PR branch** under the `github-actions[bot]` identity. This lets the compat harness run against the bundles the PR actually needs. Practical implications (force-push etiquette, fork-PR handling, orphan GC) live in [CONTRIBUTING.md](CONTRIBUTING.md#ci-writes-to-your-pr-branch). Full design: [`docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md`](docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md).

## License

MIT — see [LICENSE](LICENSE).
