# shivammathur/setup-php@v2 Compatibility Matrix

This document is the authoritative baseline for what `buildrush/setup-php` must
accept, default, and produce in order to serve as a drop-in replacement for
`shivammathur/setup-php@v2`. Every row cites a specific source line or package
so a later task encoding the behavior in Go can verify the value.

## Pinning

| Field | Value |
| --- | --- |
| Upstream repo | `shivammathur/setup-php` |
| Ref | annotated tag `v2` |
| Tag object | `728c6c6b8cf02c2e48117716a91ee48313958a19` |
| Commit SHA (dereferenced) | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Commit authored | `2026-03-15T16:14:02Z` |
| Commit message | `Update dependencies` |
| `ondrej/php` PPA, release URL | https://launchpad.net/~ondrej/+archive/ubuntu/php |
| PPA snapshot `Date:` observed (Ubuntu 24.04 noble `InRelease`) | `Sat, 11 Apr 2026 10:46:03 UTC` |
| Audit date | `2026-04-17` |

Throughout this document `<SHA>` refers to
`accd6127cb78bee3e8082180cb391013d204ef9f` and `<PPA_DATE>` refers to
`2026-04-11`.

Raw source root:
`https://raw.githubusercontent.com/shivammathur/setup-php/<SHA>/`

---

## 1. Inputs

### 1.1 `action.yml` declared inputs

Source: `action.yml` @ `<SHA>` (lines 1-38).

| Input | Required | Default | Description (verbatim) | Source |
| --- | --- | --- | --- | --- |
| `php-version` | false | *(unset)* | `Setup PHP version.` | `action.yml` L8-10 |
| `php-version-file` | false | *(unset)* | `Setup PHP version from a file.` | `action.yml` L11-13 |
| `extensions` | false | *(unset)* | `Setup PHP extensions.` | `action.yml` L14-16 |
| `ini-file` | false | `production` | `Set base ini file.` | `action.yml` L17-20 |
| `ini-values` | false | *(unset)* | `Add values to php.ini.` | `action.yml` L21-23 |
| `coverage` | false | *(unset)* | `Setup code coverage driver.` | `action.yml` L24-26 |
| `tools` | false | *(unset)* | `Setup popular tools globally.` | `action.yml` L27-29 |
| `github-token` | false | `${{ github.token }}` | `GitHub token to use for authentication.` | `action.yml` L30-32 |

Notes:

- `action.yml` L20 sets `ini-file` default to `'production'`; `src/utils.ts`
  `parseIniFile` (L88-97) additionally accepts `development`, `none`,
  `php.ini-production` and `php.ini-development`, and falls back to
  `production` for everything else.
- `src/utils.ts` `parseVersion` (L63-81) treats `latest`, `lowest`, `highest`,
  `nightly`, `master` and the pattern `<digit>.x` as manifest lookups against
  `src/configs/php-versions.json`. Any other value >=2 chars is truncated to
  its first three characters (e.g. `8.4.3` → `8.4`); single-char numeric
  input becomes `<n>.0`.
- `src/install.ts` L37 runs `getScript`; the default `ini-file` value
  `production` is what ultimately passes into `configure_php` in
  `src/scripts/linux.sh`.

### 1.2 Env-var-driven inputs (not declared in `action.yml`)

`src/utils.ts` `getInput` (L30-46) first consults `core.getInput(name)` and
falls back to `readEnv(name)` (L11-22), which probes
`$NAME`, `$name`, `$NAME_UPPER`, `$name-with-dashes`, `$NAME-WITH-DASHES`.
In addition, `src/scripts/unix.sh` `read_env` (L52-85) consumes several
lowercase/uppercase flag pairs directly. The following are supported but not
exposed in `action.yml`:

| Variable (primary) | Aliases | Semantics | Default | Source |
| --- | --- | --- | --- | --- |
| `fail-fast` / `fail_fast` | `FAIL_FAST` | If `true`, `add_log` aborts on any `✗` row. | `false` | `src/install.ts` L54; `src/scripts/unix.sh` L38, L56, L79 |
| `update` | `UPDATE` | Force-update PHP to latest patch. | `false` (auto `true` if Ondrej PPA is missing on a GitHub-hosted Ubuntu runner, `src/scripts/unix.sh` L69-77). | `src/scripts/unix.sh` L53, L71, L73, L81 |
| `debug` | `DEBUG` | `true` → install `-dbgsym` packages and `update=true`. | `false` | `src/scripts/unix.sh` L54; `src/scripts/linux.sh` L98, L203-205 |
| `phpts` | `PHPTS` | `ts` / `zts` → install thread-safe build (also forces `update=true`). | `nts` | `src/scripts/unix.sh` L55 |
| `runner` | `RUNNER` | Overrides auto-detected `github` / `self-hosted`. | `github` on GitHub Actions image, else `self-hosted`. | `src/scripts/unix.sh` L57-67 |
| `setup_php_tools_dir` | `SETUP_PHP_TOOLS_DIR` | Where tool shims are placed. | `/usr/local/bin` | `src/scripts/unix.sh` L61 |
| `setup_php_tool_cache_dir` | `SETUP_PHP_TOOL_CACHE_DIR` | Where downloaded tools are cached. | `${RUNNER_TOOL_CACHE:-/opt/hostedtoolcache}/setup-php/tools` | `src/scripts/unix.sh` L62 |
| `GITHUB_TOKEN` | — | Consumed verbatim; `src/install.ts` L55 sets it from the `github-token` input if unset. | *(populated)* | `src/install.ts` L55 |
| `COMPOSER_PROJECT_DIR` | — | Directory searched for `composer.lock` / `composer.json` when no explicit version is supplied. | `""` (cwd via `path.join`) | `src/utils.ts` L454-480 |
| `COMPOSER_TOKEN` | — | Preferred over `GITHUB_TOKEN` for Composer's GitHub OAuth; `setup-php` writes it to `auth.json`. Documented as deprecated but still honored. | *(unset)* | `src/scripts/tools/add_tools.sh` L86; `src/tools.ts` L113; README L817 |
| `PACKAGIST_TOKEN` | — | Private Packagist http-basic password, written to `auth.json`. | *(unset)* | `src/scripts/tools/add_tools.sh` L83-84; README L823 |
| `COMPOSER_AUTH_JSON` | — | Raw JSON blob tee'd into `$COMPOSER_HOME/auth.json`. | *(unset)* | `src/scripts/tools/add_tools.sh` L75-81; README L836 |
| `COMPOSER_PROCESS_TIMEOUT` | — | Overwrites the bundled `composer.env` default (`0`). | `0` | `src/scripts/tools/add_tools.sh` L104-107; `src/configs/composer.env` |
| `COMPOSER_ALLOW_PLUGINS` | — | Comma list of plugin package globs; each is set to `allow-plugins.<pkg>=true` via `composer global config`. | *(unset)* | `src/scripts/tools/add_tools.sh` L109-111 |
| `RUNNER_TOOL_CACHE` | — | Reassigned to `/opt/hostedtoolcache` for self-hosted runners. | GitHub Actions sets it. | `src/scripts/unix.sh` L228 |
| `ImageOS`, `ImageVersion`, `RUNNER_ENVIRONMENT`, `ACT`, `CONTAINER` | — | Heuristics for self-hosted detection. | *(GitHub sets or unset)* | `src/scripts/unix.sh` L57-59 |

`src/configs/composer.env` unconditionally exports the following to
`$GITHUB_ENV` / the profile (see `add_env_path` in `src/scripts/unix.sh`
L187-196 and `configure_composer` in `add_tools.sh`):

```
COMPOSER_PROCESS_TIMEOUT=0
COMPOSER_NO_INTERACTION=1
COMPOSER_NO_AUDIT=1
```

`COMPOSER_ALLOW_SUPERUSER` is **not** exported by v2 (grep confirmed absent
from `src/scripts/**`). The plan mentions it as a candidate; we record its
absence explicitly so downstream tasks do not assume otherwise.

### 1.3 Outputs

| Output | Value | Source |
| --- | --- | --- |
| `php-version` | PHP version in semver (e.g. `8.4.20`), via `set_output "php-version" "$semver"`. | `action.yml` L33-35; `src/scripts/linux.sh` L326; `src/scripts/unix.sh` L260-262 |

### 1.4 Precedence rules for `php-version`

From `src/utils.ts` `readPHPVersion` (L437-483):

1. `php-version` input (if non-empty).
2. The file named by `php-version-file`, or `.php-version` if that input is
   empty. The file is parsed with the regex `^(?:php\s)?(\d+\.\d+\.\d+)$`
   (i.e. plain version or leading `php ` — compatible with `asdf`'s
   `.tool-versions`); if the regex does not match the file's raw trimmed
   content is returned.
3. If `php-version-file` was explicitly set but the file does not exist,
   `throw new Error("Could not find '${versionFile}' file.")`.
4. `${COMPOSER_PROJECT_DIR}/composer.lock` → `platform-overrides.php`.
5. `${COMPOSER_PROJECT_DIR}/composer.json` → `config.platform.php`.
6. Fallback literal `latest` (resolved via `src/configs/php-versions.json`
   manifest in `parseVersion`, L63-81).

---

## 2. Default `php.ini` values applied on Linux

`src/scripts/unix.sh` `configure_php` (L247-257) is called unconditionally
from `src/scripts/linux.sh` `setup_php` (L325). It concatenates
`src/configs/ini/php.ini` into every `php.ini` file discovered under
`$ini_dir/..` (i.e. every SAPI). It additionally appends
`src/configs/ini/xdebug.ini` when the PHP version matches
`xdebug3_versions` (`7.[2-4]|8.[0-9]`) and
`src/configs/ini/jit.ini` (or `jit_aarch64.ini` on arm64/aarch64) to the
per-SAPI `99-pecl.ini` scan-dir file when the version matches
`jit_versions` (`8.[0-9]`).

### 2.1 `src/configs/ini/php.ini` (unconditional on Linux/Darwin)

| Key | Value | Source |
| --- | --- | --- |
| `date.timezone` | `UTC` | `src/configs/ini/php.ini` L1 |
| `memory_limit` | `-1` | `src/configs/ini/php.ini` L2 |

### 2.2 `src/configs/ini/xdebug.ini` (applied when PHP version matches `7.[2-4]|8.[0-9]`)

| Key | Value | Condition | Source |
| --- | --- | --- | --- |
| `xdebug.mode` | `coverage` | Version matches `xdebug3_versions` — PHP 7.2 through 8.9. | `src/configs/ini/xdebug.ini` L1; `src/scripts/unix.sh` L9, L254 |

Note: this line is written even if xdebug is not installed. It is a no-op in
that case because PHP ignores ini keys for unloaded extensions.

### 2.3 `src/configs/ini/jit.ini` (applied when PHP version matches `8.[0-9]`)

| Key | Value | Condition | Source |
| --- | --- | --- | --- |
| `opcache.enable` | `1` | PHP 8.0–8.9 (`jit_versions` regex). | `src/configs/ini/jit.ini` L1; `src/scripts/unix.sh` L8, L256 |
| `opcache.jit_buffer_size` | `256M` | PHP 8.0–8.9. | `src/configs/ini/jit.ini` L2 |
| `opcache.jit` | `1235` | PHP 8.0–8.9. | `src/configs/ini/jit.ini` L3 |

The JIT block is written to `$pecl_file` (`$scan_dir/99-pecl.ini`) — not to
the main `php.ini` — so user `ini-values` applied later (which tee into
`${pecl_file:-${ini_file[@]}}`, per `src/config.ts` L20) override or append on
top of it.

### 2.4 `src/configs/ini/jit_aarch64.ini` (arm64/aarch64 only)

Audited 2026-04-21 against pinned SHA `accd6127cb78bee3e8082180cb391013d204ef9f`:

| Key                       | Value  |
| ------------------------- | ------ |
| `opcache.enable`          | `1`    |
| `opcache.jit_buffer_size` | `128M` |
| `opcache.jit`             | `1235` |

Only `opcache.jit_buffer_size` diverges from x86_64's §2.3 (`256M`). Encoded
in `internal/compat.DefaultIniValues(phpVersion, arch)` by slice C1.

Source: https://raw.githubusercontent.com/shivammathur/setup-php/accd6127cb78bee3e8082180cb391013d204ef9f/src/configs/ini/jit_aarch64.ini

### 2.5 Base ini file selection

`src/scripts/linux.sh` `add_php_config` (L264-283):

- `production` (default): copies `php.ini-production` over every SAPI
  `php.ini`. If `php.ini-production.cli` exists in `/usr/lib/php/<ver>/` it
  also overwrites the CLI-scoped `php.ini`.
- `development`: copies `php.ini-development`.
- `none`: truncates every SAPI `php.ini` to empty.
- The chosen mode is recorded in `/usr/lib/php/<ver>/php.ini-current` so
  repeated invocations are idempotent.

### 2.6 Extension-priority overrides

`src/configs/mod_priority` maps extension names to the numeric prefix used
when generating `/etc/php/<ver>/mods-available/<ext>.ini` files (the default
priority is `20`, per `src/scripts/extensions/add_extensions.sh` L57). Content
at `<SHA>`:

| Extension | Priority | Notes |
| --- | --- | --- |
| `mysqlnd`, `opcache`, `pdo` | `10` | Must load early. |
| `psr`, `xml` | `15` | |
| `apc`, `apcu_bc`, `apcu-bc`, `http`, `pecl_http`, `pecl-http`, `mailparse`, `memcached`, `openswoole`, `swoole` | `25` | |
| `blackfire`, `couchbase`, `decimal`, `ds`, `event`, `ev`, `grpc`, `inotify`, `maxminddb`, `mysqlnd_ms`, `protobuf`, `rdkafka`, `vips`, `zstd` | `30` | |
| `phalcon` | `35` | |
| `libvirt-php` | `40` | |

Complete raw file: `https://raw.githubusercontent.com/shivammathur/setup-php/<SHA>/src/configs/mod_priority` (31 entries).

---

## 3. Bundled extensions by PHP version (Ondrej PPA on Ubuntu 24.04 noble)

Each list below is the exact output of `phpX.Y -m` after installing only
`phpX.Y-cli` from `ppa:ondrej/php` inside a fresh `ubuntu:24.04` container on
2026-04-17. The list is authoritative for what you get from a clean
`phpX.Y-cli` install on noble — it does not include packages like `php-json`
or `php-mbstring` which v2 commonly adds via `phpX.Y-<ext>` packages.

The entries `[Zend Modules]` section contains only `Zend OPcache` for every
version (reproduced once below).

Methodology:

```
docker run --rm ubuntu:24.04 bash -c '
  DEBIAN_FRONTEND=noninteractive apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq software-properties-common ca-certificates
  add-apt-repository -y ppa:ondrej/php
  DEBIAN_FRONTEND=noninteractive apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends phpX.Y-cli
  phpX.Y -m
  dpkg -s phpX.Y-cli | grep -E "^(Package|Version):"
'
```

Raw capture files: `/tmp/v2/modules/php{8.1,8.2,8.3,8.4,8.5}.txt` (this
workstation, regenerable at will).

### 3.1 PHP 8.1 — package `8.1.34-3+ubuntu24.04.1+deb.sury.org+1`

```
calendar, Core, ctype, date, exif, FFI, fileinfo, filter, ftp, gettext, hash,
iconv, json, libxml, openssl, pcntl, pcre, PDO, Phar, posix, readline,
Reflection, session, shmop, sockets, sodium, SPL, standard, sysvmsg, sysvsem,
sysvshm, tokenizer, Zend OPcache, zlib
```

### 3.2 PHP 8.2 — package `8.2.30-3+ubuntu24.04.1+deb.sury.org+1`

```
calendar, Core, ctype, date, exif, FFI, fileinfo, filter, ftp, gettext, hash,
iconv, json, libxml, openssl, pcntl, pcre, PDO, Phar, posix, random, readline,
Reflection, session, shmop, sockets, sodium, SPL, standard, sysvmsg, sysvsem,
sysvshm, tokenizer, Zend OPcache, zlib
```

Delta vs 8.1: `+random` (core, PHP 8.2+).

### 3.3 PHP 8.3 — package `8.3.30-3+ubuntu24.04.1+deb.sury.org+1`

Same list as PHP 8.2 (identical `-m` output).

### 3.4 PHP 8.4 — package `8.4.20-1+ubuntu24.04.1+deb.sury.org+1`

Same list as PHP 8.3.

### 3.5 PHP 8.5 — package `8.5.5-1+ubuntu24.04.1+deb.sury.org+1`

```
calendar, Core, ctype, date, exif, FFI, fileinfo, filter, ftp, gettext, hash,
iconv, json, lexbor, libxml, openssl, pcntl, pcre, PDO, Phar, posix, random,
readline, Reflection, session, shmop, sockets, sodium, SPL, standard, sysvmsg,
sysvsem, sysvshm, tokenizer, uri, Zend OPcache, zlib
```

Delta vs 8.4: `+lexbor`, `+uri` (both new in PHP 8.5).

### 3.6 Summary table

| Extension | 8.1 | 8.2 | 8.3 | 8.4 | 8.5 |
| --- | --- | --- | --- | --- | --- |
| calendar | ✓ | ✓ | ✓ | ✓ | ✓ |
| Core | ✓ | ✓ | ✓ | ✓ | ✓ |
| ctype | ✓ | ✓ | ✓ | ✓ | ✓ |
| date | ✓ | ✓ | ✓ | ✓ | ✓ |
| exif | ✓ | ✓ | ✓ | ✓ | ✓ |
| FFI | ✓ | ✓ | ✓ | ✓ | ✓ |
| fileinfo | ✓ | ✓ | ✓ | ✓ | ✓ |
| filter | ✓ | ✓ | ✓ | ✓ | ✓ |
| ftp | ✓ | ✓ | ✓ | ✓ | ✓ |
| gettext | ✓ | ✓ | ✓ | ✓ | ✓ |
| hash | ✓ | ✓ | ✓ | ✓ | ✓ |
| iconv | ✓ | ✓ | ✓ | ✓ | ✓ |
| json | ✓ | ✓ | ✓ | ✓ | ✓ |
| lexbor | | | | | ✓ |
| libxml | ✓ | ✓ | ✓ | ✓ | ✓ |
| openssl | ✓ | ✓ | ✓ | ✓ | ✓ |
| pcntl | ✓ | ✓ | ✓ | ✓ | ✓ |
| pcre | ✓ | ✓ | ✓ | ✓ | ✓ |
| PDO | ✓ | ✓ | ✓ | ✓ | ✓ |
| Phar | ✓ | ✓ | ✓ | ✓ | ✓ |
| posix | ✓ | ✓ | ✓ | ✓ | ✓ |
| random | | ✓ | ✓ | ✓ | ✓ |
| readline | ✓ | ✓ | ✓ | ✓ | ✓ |
| Reflection | ✓ | ✓ | ✓ | ✓ | ✓ |
| session | ✓ | ✓ | ✓ | ✓ | ✓ |
| shmop | ✓ | ✓ | ✓ | ✓ | ✓ |
| sockets | ✓ | ✓ | ✓ | ✓ | ✓ |
| sodium | ✓ | ✓ | ✓ | ✓ | ✓ |
| SPL | ✓ | ✓ | ✓ | ✓ | ✓ |
| standard | ✓ | ✓ | ✓ | ✓ | ✓ |
| sysvmsg | ✓ | ✓ | ✓ | ✓ | ✓ |
| sysvsem | ✓ | ✓ | ✓ | ✓ | ✓ |
| sysvshm | ✓ | ✓ | ✓ | ✓ | ✓ |
| tokenizer | ✓ | ✓ | ✓ | ✓ | ✓ |
| uri | | | | | ✓ |
| Zend OPcache | ✓ | ✓ | ✓ | ✓ | ✓ |
| zlib | ✓ | ✓ | ✓ | ✓ | ✓ |

Caveats:

- The list is what `phpX.Y-cli` alone ships on Ubuntu 24.04 noble. Ondrej
  builds for Debian and older Ubuntu series differ (e.g. some extensions are
  shared packages on focal but built-in on noble). This audit intentionally
  scopes to noble because that is what `ubuntu-latest` runners currently use.
- v2 on GitHub-hosted runners typically starts from a pre-installed PHP whose
  bundled set can exceed the above; this document is the Ondrej-only baseline
  that `buildrush/setup-php` OCI bundles must match or exceed.

---

## 4. Extension-list syntax

Source: `src/extensions.ts` (Linux branch `addExtensionLinux`, L244-353) and
`src/utils.ts` `extensionArray` (L213-237).

### 4.1 Tokenization

`extensionArray` (L221-236) splits the csv, lowercases each entry, and then
for non-source tokens strips the leading `:`/`php-`/`php_`/`none`/`zend ` and
collapses `pdo-mysql-X` → `pdo-mysqlX` style prefixes via the regex
`/^(:)?(php[-_]|none|zend )|(-[^-]*)-/`. If a literal `none` token appears
anywhere in the csv it is hoisted to the front of the resulting array so
`disable_all_shared` runs before any add_extension.

### 4.2 Accepted tokens

| Token shape | Effect on Linux | Code path (`src/extensions.ts`) |
| --- | --- | --- |
| `mbstring` (bare) | `add_extension mbstring extension` → `apt-get install phpX.Y-mbstring` with PECL fallback. | default branch, L350 |
| `sqlite` | Rewritten to `sqlite3` and then enabled as bare. | L344-346 |
| `pdo_mysql`, `pdo-mysql` | Routed through `add_pdo_extension mysql`; enables `mysqlnd` + `mysqli` + `pdo_mysql`. | L339-342 |
| `pdo_firebird` / `pdo_dblib` / `pdo_sqlite` | `add_pdo_extension <db>` with special-casing for `firebird` (installs `libfbclient2`), `dblib` (→ `sybase`), `sqlite` (→ `sqlite3`). | L339-342; `src/scripts/linux.sh` L58-86 |
| `:mbstring` | Disables extension after all adds run (collected into `remove_script`, appended last). | L260-262 |
| `none` | Inserts `disable_all_shared` — truncates `extension=`/`zend_extension=` lines from all `php.ini` files and `99-pecl.ini`. | L264-266; `add_extensions.sh` L124-133 |
| `ext-<semver>` (e.g. `xdebug-3.1.6`) | `add_pecl_extension xdebug 3.1.6 zend_extension`. | L317-323 |
| `ext-<state>` where state ∈ `stable\|beta\|alpha\|devel\|snapshot\|rc\|preview` (e.g. `xdebug-beta`) | `add_unstable_extension xdebug beta zend_extension`. | L308-314 |
| `ext-owner/repo@ref` or `ext-<url>://<host>/owner/repo@ref` (e.g. `amqp-git/php-amqp@master`) | `add_extension_from_source` — clones, patches, `phpize && make && make install`. | L268-270; `src/utils.ts` L418-432 |
| `blackfire`, `blackfire-<semver>` | `customPackage` → sources `src/scripts/extensions/blackfire.sh`. | L282-305 |
| `relay`, `relay-<semver>`, `relay-nightly` | Same (PHP 7.4–8.5 only). | L279-281 |
| `couchbase`, `event`, `gearman`, `geos`, `ibm_db2`, `pdo_ibm`, `pdo_oci`, `oci8`, `http`, `pecl_http`, `pdo_firebird` | `customPackage` via matching per-extension script. | L288-290 |
| `intl-<icu-version>` (e.g. `intl-74.2`, excluded on PHP 5.3–5.5) | `customPackage intl intl-74.2 linux` — sources `intl.sh`. | L291 |
| `ioncube` | `customPackage` (PHP 5.3–8.5). | L292 |
| `phalcon3` / `phalcon4` / `phalcon5` (version-gated) | `customPackage`. | L293 |
| `sqlsrv`, `pdo_sqlsrv` (PHP 7+) | `customPackage sqlsrv`. | L296 |
| `zephir_parser[-semver]` (PHP 7.0–8.5) | `customPackage`. | L297-299 |
| `cubrid` / `pdo_cubrid` | `customPackage` (PHP 5.3–7.x only). | L285-287 |
| `pcov` (PHP 5.3–7.0) | Logs "not supported" and skips. | L326-328 |
| `xdebug2` (PHP 7.2–7.4) | Forces `add_pecl_extension xdebug 2.9.8 zend_extension`. | L330-337 |

`src/utils.ts` `getExtensionPrefix` (L270-277) determines whether the
extension is loaded with `extension=` or `zend_extension=`; extensions
matching `/xdebug([2-3])?$|opcache|ioncube|eaccelerator/` use
`zend_extension`, all others use `extension`.

### 4.3 Transformation examples

Input → resulting `add_*` calls (Linux, PHP 8.4):

- `mbstring, intl` → `add_extension mbstring extension` + `add_extension intl extension`.
- `mbstring, :opcache` → `add_extension mbstring extension` first, then at the
  end `disable_extension opcache`.
- `none, mbstring` → `disable_all_shared` hoisted first, then
  `add_extension mbstring extension`.
- `xdebug-3.3.1` → `add_pecl_extension xdebug 3.3.1 zend_extension`.
- `xdebug-beta` → `add_unstable_extension xdebug beta zend_extension`.
- `pdo_mysql` → `add_pdo_extension mysql` (which internally enables
  `mysqlnd`, `mysqli`, `pdo_mysql`).
- `blackfire-2.4.2` → source `src/scripts/extensions/blackfire.sh` then
  `add_blackfire blackfire-2.4.2`.
- `amqp-php-amqp/php-amqp@master` → `add_extension_from_source amqp https://github.com php-amqp php-amqp master extension`.
- `sqlite` → rewritten to `sqlite3` and `add_extension sqlite3 extension`.

### 4.4 `ini-values` csv parsing

`src/utils.ts` `CSVArray` (L245-263) splits on commas outside balanced
`"`/`'` pairs. It also auto-quotes RHS values containing bash-unsafe metachars
(`?{}|&~![()^`), e.g. `xdebug.mode=develop,coverage` becomes
`xdebug.mode='develop,coverage'`, and collapses `key=a=b` to `key='a=b'`. Each
resulting line is tee'd into `${pecl_file:-${ini_file[@]}}` (a.k.a.
`$scan_dir/99-pecl.ini`), per `src/config.ts` L17-22.

---

## 5. L5 Behavioral quirks beyond the input surface

Disposition key:

- **implement** — required for v2 parity in this slice.
- **document** — will be a knowing deviation documented for users.
- **follow-up** — deferred to a later issue/task; not in scope for this slice.

| # | Quirk | Source | Disposition |
| --- | --- | --- | --- |
| 5.1 | **Composer auto-install**: `filterList` in `src/tools.ts` L223-239 unconditionally `unshift`s `composer` onto the tools list unless `tools: none` is explicitly passed (`addTools` L666-667). Result: every v2 run produces `/usr/local/bin/composer` and exports `$COMPOSER_HOME`. | `src/tools.ts` L223-239, L660-719 | **implement** |
| 5.2 | **`tools: none` short-circuit**: when the sole token is literally `none`, `addTools` returns an empty string — no tools step, *including no composer install*. Important edge case for parity. | `src/tools.ts` L666-667 | **implement** |
| 5.3 | **Composer env exports** (`src/configs/composer.env`): `COMPOSER_PROCESS_TIMEOUT=0`, `COMPOSER_NO_INTERACTION=1`, `COMPOSER_NO_AUDIT=1` are tee'd into `$GITHUB_ENV` by `add_env_path` so all subsequent steps inherit them. | `src/configs/composer.env`; `src/scripts/unix.sh` L187-196; `src/scripts/tools/add_tools.sh` `set_composer_env` L102-112 | **implement** |
| 5.4 | **`COMPOSER_ALLOW_SUPERUSER` is _not_ set** by v2 (grepped absent). The plan's mention is defensive; record as "not required". | absence of grep match in `src/**` | **document** |
| 5.5 | **Composer auth**: if `COMPOSER_AUTH_JSON` env is valid JSON, it overwrites `$COMPOSER_HOME/auth.json`. Else `PACKAGIST_TOKEN` adds an `http-basic: repo.packagist.com` entry and `COMPOSER_TOKEN` (fallback `GITHUB_TOKEN`) adds a `github-oauth: github.com` entry. | `src/scripts/tools/add_tools.sh` L73-100 | **follow-up** (later slice covers private-registry auth) |
| 5.6 | **`coverage: none` side-effect**: disables both Xdebug and PCOV via `disableCoverage` which appends `:pcov:false, :xdebug:false` to the extensions pass. Even if the user did not ask for xdebug, v2 removes it from the loaded set. | `src/coverage.ts` L105-118, L144-145 | **implemented (slice #2 — compat closeout)** |
| 5.7 | **`coverage: xdebug` side-effect**: auto-disables pcov and installs `xdebug` (mapping `xdebug3` → `xdebug` after version gating). Sets up `xdebug.mode=coverage` via the `xdebug.ini` default (§2.2). | `src/coverage.ts` L26-52, L138-143; `src/configs/ini/xdebug.ini` | **implemented (slice #2 — compat closeout)** |
| 5.8 | **`coverage: pcov`**: auto-disables xdebug, installs pcov, writes `pcov.enabled=1` as an additional ini value. | `src/coverage.ts` L61-96 | **implemented (slice #2 — compat closeout)** |
| 5.9 | **`xdebug2` alias**: on PHP 7.2–7.4 the token `xdebug2` is pinned to PECL `xdebug 2.9.8`. Attempting `xdebug2` on 8.x is rejected with a `✗` log (not fatal unless `fail-fast=true`). | `src/coverage.ts` L9-16; `src/extensions.ts` L330-337 | **follow-up** (phase 2 does not ship PHP < 8.1) |
| 5.10 | **`fail-fast` mode**: when truthy, `add_log` exits `1` on the first `✗`. Otherwise errors are informational. | `src/scripts/unix.sh` L30-40 | **implement** |
| 5.11 | **`update: true`**: forces an `apt-get install` of the latest patch even if a compatible PHP is already on the runner. Auto-enabled when `phpts=ts` or `debug=true`, or when `ppa:ondrej/php` is absent from a GitHub-hosted Ubuntu runner. | `src/scripts/unix.sh` L53-77; `src/scripts/linux.sh` `update_php`/`add_php` L216-242 | **document** (OCI bundles always provide a specific patch, so "update" maps to "pick a newer bundle tag" in our model) |
| 5.12 | **`debug: true`**: installs `phpX.Y-<ext>-dbgsym` for every extension and forces `update=true`. | `src/scripts/linux.sh` L98, L203-205 | **follow-up** |
| 5.13 | **`phpts: ts\|zts`**: switches to `setup_php_builder` from `shivammathur/php-builder` releases. | `src/scripts/unix.sh` L55; `src/scripts/linux.sh` L115-118, L229-236 | **follow-up** |
| 5.14 | **Tool cache location**: `tool_path_dir` default `/usr/local/bin`; `tool_cache_path_dir` default `${RUNNER_TOOL_CACHE:-/opt/hostedtoolcache}/setup-php/tools`. Overridable via `setup_php_tools_dir` / `setup_php_tool_cache_dir`. | `src/scripts/unix.sh` L61-62 | **implement** |
| 5.15 | **`php-version-file` precedence**: input > default `.php-version` > `${COMPOSER_PROJECT_DIR}/composer.lock#platform-overrides.php` > `${COMPOSER_PROJECT_DIR}/composer.json#config.platform.php` > literal `latest`. An explicitly named file that doesn't exist throws; the default `.php-version` being absent is non-fatal. `.php-version` parsing supports both raw `8.4.3` and the `asdf` `.tool-versions` shape `php 8.4.3`. | `src/utils.ts` L437-483 | **implement** |
| 5.16 | **`php-version: pre-installed`**: detects `command -v php-config`, adopts that major.minor, and sets `update=false`. Missing pre-installed PHP + `pre-installed` + no `update=true` is a fatal error. | `src/scripts/unix.sh` `check_pre_installed` L233-244 | **follow-up** |
| 5.17 | **`ini-file: none`**: truncates every SAPI `php.ini` to empty before the configured defaults (§2.1–2.3) are appended. Users expect a literally empty baseline on top of which the v2 defaults still land. | `src/scripts/linux.sh` L279-281 | **implemented (slice #2 — compat closeout)** |
| 5.18 | **`$php_dir/php.ini-current` idempotency marker**: v2 records the selected `ini-file` mode so rerunning with the same mode is a no-op. | `src/scripts/linux.sh` L267-283 | **document** (our OCI model is stateless; not required) |
| 5.19 | **Extension CSV preprocessing**: lowercase, strip `php-` / `php_` / `zend ` prefixes, hoist `none`, dedupe later via `enable_extension` guard. Users rely on being able to pass `Zend OPcache` → `opcache`. | `src/utils.ts` L213-237 | **implement** |
| 5.20 | **PPA bootstrap**: `add_extension_helper` calls `add_ppa ondrej/php` before installing the apt package. On a container without the PPA (e.g. custom self-hosted), v2 adds the PPA automatically. Our OCI bundles bypass the PPA entirely. | `src/scripts/linux.sh` L97 | **document** |
| 5.21 | **PECL fallback for unknown extensions**: if the matching `phpX.Y-<ext>` apt package does not exist, `add_extension_helper` falls through to `pecl_install`. Users rely on this for extensions not packaged by Ondrej. | `src/scripts/linux.sh` L99 | **follow-up** |
| 5.22 | **Sponsor log line**: every run ends with `add_log tick setup-php https://setup-php.com/sponsor`. Pure cosmetics. | `src/install.ts` L42-43 | **document** (we do not replicate) |
| 5.23 | **Default `date.timezone=UTC`, `memory_limit=-1`** on Linux (see §2.1). Users and downstream CI scripts commonly rely on these being present. | `src/configs/ini/php.ini` | **implemented (slice #2 — compat closeout)** |

---

## References

- Pinned source root:
  https://github.com/shivammathur/setup-php/tree/accd6127cb78bee3e8082180cb391013d204ef9f
- Raw `action.yml`:
  https://raw.githubusercontent.com/shivammathur/setup-php/accd6127cb78bee3e8082180cb391013d204ef9f/action.yml
- Launchpad PPA:
  https://launchpad.net/~ondrej/+archive/ubuntu/php
- PPA `InRelease` date observed in this audit: `Sat, 11 Apr 2026 10:46:03 UTC`.

## Deviations allowlist

The compat harness (`.github/workflows/compat-harness.yml`) consults this block
when diffing our setup-php against `shivammathur/setup-php@v2`. Each entry
tolerates a specific JSON path deviation for the listed fixtures. Adding an
entry requires a `reason` and is reviewable in the PR diff that introduces the
deviation.

- `kind: ignore` — the path is dropped from both probes before comparison.
- `kind: allow` — both sides may hold different values, but both must be non-empty.

<!-- compat-harness:deviations:start -->
```yaml
deviations:
  # env_delta: v2 auto-installs composer on CI runners and sets COMPOSER_* env
  # vars. We don't install composer in Phase 2. `allow` instead of `ignore` so
  # we still catch the case where we fail to add ANY env vars.
  - path: env_delta
    kind: allow
    reason: 'v2 auto-installs composer and sets COMPOSER_* env vars; tools parity is Phase 3 scope'
    fixtures: ['*']

  # extensions: v2 on Ondrej PPA ships ~80 extensions; we ship the Phase 1
  # baseline of ~52 (core + 4 PECLs). `allow` so we still catch the case where
  # our bundle fails to load any extensions.
  - path: extensions
    kind: allow
    reason: 'bundled extension set expansion is Phase 2 follow-up slice #3 (top-50)'
    fixtures: ['*']

  # path_additions: both sides add a tool-chain directory to PATH, but the
  # specific paths differ by architecture (our bundled install vs v2's
  # hostedtoolcache + composer vendor/bin). `allow` requires both non-empty.
  - path: path_additions
    kind: allow
    reason: 'both sides prepend a PHP install prefix; exact directory differs by design'
    fixtures: ['*']

  # Opcache defaults from php.ini-production: v2 ships php.ini-production; our
  # builder now does too (T5). These four keys come from the production preset
  # and should match once the 8.4 core is rebuilt. Pending post-merge CI
  # confirmation before removing.
  - path: ini.opcache.enable_cli
    kind: ignore
    reason: 'php.ini-production default; pending post-merge harness confirmation'
    fixtures: ['*']
  - path: ini.opcache.memory_consumption
    kind: ignore
    reason: 'php.ini-production default; pending post-merge harness confirmation'
    fixtures: ['*']
  - path: ini.opcache.revalidate_freq
    kind: ignore
    reason: 'php.ini-production default; pending post-merge harness confirmation'
    fixtures: ['*']
  - path: ini.opcache.validate_timestamps
    kind: ignore
    reason: 'php.ini-production default; pending post-merge harness confirmation'
    fixtures: ['*']

  # xdebug.mode / start_with_request on multi-ext: v2's apt xdebug install is
  # not reliably loaded on ubuntu-24.04 runners, so theirs often reports empty
  # for both keys. Our xdebug 3.5.1 loads cleanly from the PECL bundle; when
  # loaded without coverage: driving the install, xdebug sets start_with_request
  # to its compile-time default ("default"). This divergence is inherent to
  # v2's best-effort extension install — not fixable on our side.
  - path: ini.xdebug.mode
    kind: ignore
    reason: 'v2 apt xdebug install unreliable on ubuntu-24.04; theirs often unloaded'
    fixtures: ['multi-ext*']
  - path: ini.xdebug.start_with_request
    kind: ignore
    reason: 'xdebug version-default drift (v2 apt unreliable; ours = xdebug 3.5.1 default)'
    fixtures: ['multi-ext*']

  # disable_functions: Ondrej's php8.*-cli Debian packages patch
  # php.ini-development to blacklist all pcntl_* functions (CI hardening).
  # Our source-built PHP uses the stock upstream php.ini-development which
  # has disable_functions= empty. Replicating Ondrej's patch set is out of
  # scope.
  - path: ini.disable_functions
    kind: ignore
    reason: 'Ondrej Debian patch on php.ini-development adds pcntl_* blacklist; stock upstream does not'
    fixtures: ['ini-file-development*']
```
<!-- compat-harness:deviations:end -->
