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

| Input | Description | Default |
|-------|-------------|---------|
| `php-version` | PHP version to install | `8.4` |
| `extensions` | Comma-separated extensions | — |
| `ini-values` | Comma-separated ini settings | — |
| `coverage` | Coverage driver (xdebug, pcov, none) | `none` |
| `tools` | Comma-separated tools | — |
| `php-version-file` | File containing PHP version | — |

## Outputs

| Output | Description |
|--------|-------------|
| `php-version` | Resolved PHP version (e.g., `8.4.6`) |

## Supported Matrix

| OS | Arch | PHP | Thread Safety |
|----|------|-----|---------------|
| Linux (Ubuntu 24.04) | x86_64 | 8.4 | NTS |

### Bundled Extensions

These ship inside the PHP core bundle — no extra download required:

bcmath, calendar, ctype, curl, dom, exif, filter, ftp, gd, hash, iconv, intl, json, mbstring, opcache, openssl, pdo, pdo_mysql, pdo_pgsql, pdo_sqlite, pgsql, readline, session, simplexml, soap, sockets, sodium, sqlite3, tokenizer, xml, xmlreader, xmlwriter, zip, zlib

### Separately Installable Extensions

| Extension | Version |
|-----------|---------|
| redis | 6.2.0 |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

MIT — see [LICENSE](LICENSE).
