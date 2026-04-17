# T12 Hand-off: Align PHP 8.4 Build with v2-Compat Bundled Set

**Deferred from the Phase 2 compat-first slice.** Requires local Docker iteration; not suitable for autonomous execution. This document gives a precise, small-step recipe for when you're ready.

## Goal

Make the PHP 8.4 core bundle's compiled-in extension set a **superset** of `compat.BundledExtensions("8.4")` (the v2/Ondrej PPA baseline), so every extension a v2 user expects to be available on a clean `setup-php` run is available on our action too.

Per the plan, subtractive changes are banned — our existing compiled set (mbstring, curl, intl, zip, soap, pdo_mysql, sqlite, gd, pgsql, bcmath) stays. We only **add** flags for v2-bundled extensions we don't currently compile.

## Current vs. target

Current `builders/linux/build-php.sh` produces (33 extensions after the `php -m` dedupe): the **fat set** historically in `catalog/php.yaml` pre-T11. Retain all of it.

v2/Ondrej ships these 34 (lowercase, post-normalization):

```
calendar, core, ctype, date, exif, ffi, fileinfo, filter, ftp, gettext,
hash, iconv, json, libxml, opcache, openssl, pcntl, pcre, pdo, phar,
posix, random, readline, reflection, session, shmop, sockets, sodium,
spl, standard, sysvmsg, sysvsem, sysvshm, tokenizer, zlib
```

Cross-referencing against what our current build produces (run `php -m` inside the current 8.4 bundle or consult pre-T11 `catalog/php.yaml`), these v2-bundled entries are **missing** from our compiled set:

| Missing extension | Configure flag | Notes |
|---|---|---|
| `ffi` | `--with-ffi` | Foreign Function Interface; PHP 7.4+ |
| `gettext` | `--with-gettext` | libc's gettext; needs no extra apt dep on Ubuntu |
| `pcntl` | `--enable-pcntl` | Process control; POSIX-only, fine for Linux |
| `posix` | `--enable-posix` | May already be default-on; adding flag is idempotent |
| `shmop` | `--enable-shmop` | Shared-memory ops |
| `sysvmsg` | `--enable-sysvmsg` | System V message queues |
| `sysvsem` | `--enable-sysvsem` | System V semaphores |
| `sysvshm` | `--enable-sysvshm` | System V shared memory |

Extensions in the v2-bundled list that are **already present** in our build by default (no flag change needed): `calendar` ✓, `ctype` ✓, `date` (always-on), `exif` ✓, `fileinfo` (default on), `filter` (always-on), `ftp` ✓, `hash` (always-on), `iconv` (auto-detected), `json` (always-on since 8.0), `libxml` (pulled by xml), `opcache` ✓, `openssl` ✓, `pcre` (always-on), `pdo` (always-on), `phar` (on by default), `random` (PHP 8.2+ always-on), `readline` ✓, `reflection` (always-on), `session` (always-on), `sockets` ✓, `sodium` ✓, `spl` (always-on), `standard` (always-on), `tokenizer` (always-on), `zlib` ✓, `core` (always-on).

## Concrete diff

Apply to `builders/linux/build-php.sh`, inside the `./configure` invocation (currently lines 51–77):

```diff
 ./configure \
   --prefix=/usr/local \
   --enable-mbstring \
   --with-curl \
   --with-zlib \
   --with-openssl \
   --enable-bcmath \
   --enable-calendar \
   --enable-exif \
   --enable-ftp \
   --enable-intl \
   --with-zip \
   --enable-soap \
   --enable-sockets \
   --with-pdo-mysql \
   --with-pdo-sqlite \
   --with-sqlite3 \
   --with-readline \
   --with-sodium \
   --enable-gd \
   --with-freetype \
   --with-jpeg \
   --with-webp \
   --with-pdo-pgsql \
   --with-pgsql \
+  --with-ffi \
+  --with-gettext \
+  --enable-pcntl \
+  --enable-posix \
+  --enable-shmop \
+  --enable-sysvmsg \
+  --enable-sysvsem \
+  --enable-sysvshm \
   --disable-cgi \
   --enable-opcache
```

Also update the apt-get install list (currently line 33) to include FFI and gettext dev headers:

```diff
 $SUDO apt-get install -y -qq \
   autoconf bison re2c pkg-config build-essential \
   libicu-dev libcurl4-openssl-dev libzip-dev libsqlite3-dev \
   libpq-dev libonig-dev libreadline-dev libsodium-dev \
   libfreetype6-dev libjpeg-dev libwebp-dev libxml2-dev \
-  zlib1g-dev libssl-dev gnupg2 xz-utils curl
+  zlib1g-dev libssl-dev gnupg2 xz-utils curl \
+  libffi-dev libgettextpo-dev
```

Mirror the same adds in `catalog/php.yaml` under `versions."8.4".configure_flags.common` so the planner's view matches the builder's reality.

## Verification loop

```bash
# 1. Rebuild locally
make bundle-php PHP_VERSION=8.4

# 2. Unpack the produced bundle and introspect
mkdir -p /tmp/inspect && tar -I zstd -xf /tmp/bundle.tar.zst -C /tmp/inspect
/tmp/inspect/usr/local/bin/php -m | sort -f | tee /tmp/got.txt

# 3. Derive the expected list — all lowercase, normalized
go run ./scripts/dump_bundled 8.4 | sort > /tmp/want.txt  # one-off throwaway

# 4. Diff
diff /tmp/want.txt /tmp/got.txt
```

Expected outcome: every line in `/tmp/want.txt` appears in `/tmp/got.txt`. Extra lines in `/tmp/got.txt` (the documented deviations — mbstring, curl, intl, zip, soap, pdo_mysql, sqlite3, pdo_sqlite, gd, pdo_pgsql, pgsql, bcmath) are fine.

If a `want` line is missing: iterate on the configure flags or install list and rebuild. Common failure modes:

| Missing from `got` | Likely cause | Fix |
|---|---|---|
| `ffi` | `libffi-dev` not installed | add to apt list; `./configure` will detect it |
| `gettext` | `libgettextpo-dev` missing or wrong package | try `gettext` or `libc6-dev`; on Ubuntu 24.04, stdlib gettext is part of libc |
| `pcntl` | Flag misspelled or PHP version too old | check `./configure --help | grep pcntl` |
| `posix` | PHP already compiles it by default but Makefile elided | harmless to add flag explicitly |
| any | Build error above the `make install` line | scroll back; usually missing dev header |

Once `/tmp/got.txt` is a superset of `/tmp/want.txt`, the build is aligned.

## Rebuild of PECL extension bundles

The audit did not identify an ABI-affecting change in our 8.4 core between the current compiled set and the target compiled set — adding compile-time extensions (`--enable-pcntl`, `--with-ffi`, etc.) does not change the core PHP ABI that separately-loadable `.so` files rely on. Therefore the four PECL bundles (redis, xdebug, pcov, apcu) in `bundles.lock` do **not** need to be rebuilt.

If in practice a PECL extension fails to load against the new bundle (e.g. `symbol not found` at runtime), rebuild the affected extension via `make bundle-ext EXT_NAME=<name> EXT_VERSION=<ver> PHP_ABI=8.4:linux:x86_64:nts` and update `bundles.lock`.

## Documentation updates

Once the build is aligned:

1. Update `docs/compat-matrix.md` §5 to list the **deliberate deviations** — extensions our compiled set includes that v2 does not:

   - mbstring, curl, intl, zip, soap, pdo_mysql, sqlite3, pdo_sqlite, gd (+ freetype/jpeg/webp), pdo_pgsql, pgsql, bcmath.
   
   Note that v2 provides these via `apt install phpX.Y-<name>` when listed in the `extensions:` input; our implementation makes them available unconditionally, so `extensions: none` will still have them loaded. This is a documented compat gap. Track in follow-up issue: "honor `extensions: none` beyond the catalog by disabling compile-time extras" (out of scope for this slice).

2. Update `bundles.lock` via the existing CI pipeline (merge the builder change to `main`; `build-php-core.yml` rebuilds and `update-lockfile` PR lands the new digest).

## Commit

```
git add builders/linux/build-php.sh catalog/php.yaml docs/compat-matrix.md
git commit -m "fix(builders/linux): add ffi, gettext, pcntl, posix, shmop, sysv* for v2 bundled parity"
```

Conventional commit. No AI attribution.

## Scope boundary

**In scope**: adding the 8 missing flags + 2 dev headers; re-diffing `php -m` against `compat.BundledExtensions("8.4")`; documenting deviations.

**Out of scope**:
- Removing any existing flag (banned — subtractive).
- Per-version variations (8.1, 8.2, 8.3, 8.5 builds are follow-up slices; their `configure_flags` in `catalog/php.yaml` remain unset until then).
- aarch64 (follow-up).
- ZTS variant (follow-up).
- Fixing the "extensions: none doesn't disable compile-time extras" gap (follow-up issue).

## If you want me to drive T12 once Docker is available

Tell me "resume T12", confirm Docker is running, and I'll dispatch a subagent that follows this recipe end-to-end with a 30-minute wall budget. The subagent will iterate until `php -m` matches, commit the changes, and update docs. If the iteration blows the budget, it will pause and hand back with current state so you can debug the specific library/flag that's resisting.
