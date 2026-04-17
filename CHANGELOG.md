# Changelog

## [1.2.0](https://github.com/buildrush/setup-php/compare/v1.1.2...v1.2.0) (2026-04-17)


### Features

* **action:** declare phpts, update, fail-fast, ini-file inputs for v2 compat ([4fd27fd](https://github.com/buildrush/setup-php/commit/4fd27fd7e90b4b6149d41caf98fb0d2e14641e76))
* **builders,catalog:** add ffi, gettext, pcntl, posix, shmop, sysv* for v2 baseline parity ([cb1ce9b](https://github.com/buildrush/setup-php/commit/cb1ce9b2dd978aace200f2a36497bf18008b75a7))
* **cmd:** port update-lockfile to Go against versioned catalog ([cff322a](https://github.com/buildrush/setup-php/commit/cff322ae80f29c8c494fbf415d9e27eaae36d5fe))
* **compat:** add DefaultIniValues matching v2 Linux defaults ([26190fe](https://github.com/buildrush/setup-php/commit/26190fee775aa5300587d2311dddfdc51c3d352b))
* **compat:** add per-version BundledExtensions for 8.1-8.5 ([39ee516](https://github.com/buildrush/setup-php/commit/39ee5161952466b1fccc765b3b231fbf29a0c031))
* **compat:** scaffold compat package with unimplemented-input warning ([fa67b95](https://github.com/buildrush/setup-php/commit/fa67b954aa339ff49f72d5cd0809556de7d3700d))
* **compose:** merge compat default ini values and write disable fragments ([d77f8be](https://github.com/buildrush/setup-php/commit/d77f8beb5e03f0cda4ac5426f2f5eac0405dc100))
* **lockfile:** schema v2 with per-entry spec_hash ([f5a458d](https://github.com/buildrush/setup-php/commit/f5a458d63c8cd3c381b267fea3471bee42017ccf))
* **phpup:** wire compat defaults, exclusions, warnings, and X.Y.Z output ([21385b5](https://github.com/buildrush/setup-php/commit/21385b5c658f6c0d1f0df73ff5824a904968d576))
* **planner:** filterExisting honors spec_hash drift with grandfather rule ([653c1aa](https://github.com/buildrush/setup-php/commit/653c1aa980818895cd67b521fa6398da88836bdc))
* **planner:** HashFile, PerVersionYAML, ExtensionYAML helpers + SpecHash field ([4ff1254](https://github.com/buildrush/setup-php/commit/4ff1254d205012cf93da3aa076736b874acecb5b))
* **plan:** parse update, phpts, fail-fast, ini-file v2-compat inputs ([0544649](https://github.com/buildrush/setup-php/commit/05446490d4cd00da4b8acf854657473d3a86f6f4))
* **plan:** support v2 extension syntax (:ext exclusion, none reset) ([fabb11f](https://github.com/buildrush/setup-php/commit/fabb11f074d5341a68e2bfdc35e9c8a4b42f556f))
* **resolve:** ZTS-&gt;NTS fallback and fail-fast promotion ([764e593](https://github.com/buildrush/setup-php/commit/764e593fef3e9132fb1bad6ee0a1e879c779f9e2))
* **workflows:** carry spec_hash and gate push on caller input ([eb9ef0e](https://github.com/buildrush/setup-php/commit/eb9ef0e9cabbe5acb57ebf742b93699ab6952af5))
* **workflows:** thread spec_hash through matrix and use Go lockfile-update ([c23f6aa](https://github.com/buildrush/setup-php/commit/c23f6aa39fe16f253ded28bb3ca68a8480cf8865))


### Bug Fixes

* **cmd:** reject malformed php_abi in lockfile-update splitAbi ([5b10420](https://github.com/buildrush/setup-php/commit/5b10420f4020886b5a2fe882de49b9f443cbfa6b))
* **compat,catalog:** normalize X.Y.Z to minor and mark lexicographic sort TODO ([4c23656](https://github.com/buildrush/setup-php/commit/4c23656f9661c82ed7c8e86c8aedea105716596d))
* **compat,phpup:** normalize bundled-ext casing and let include win over exclude ([16c9189](https://github.com/buildrush/setup-php/commit/16c9189e1d6cb90b0c50ef5bcfea36bf915fa30d))
* **phpup,compat:** resolve php -v via absolute path and expose build extras to runtime ([8e31dc1](https://github.com/buildrush/setup-php/commit/8e31dc1068afc72bd4594d21f0543fb7af1f5edc))


### Documentation

* **action:** tighten input descriptions to match actual behavior ([713e507](https://github.com/buildrush/setup-php/commit/713e5073bab096c8f192d2afd00c2fd1923c2d04))
* add lockfile spec-hash and PR-time build design spec ([89a79bb](https://github.com/buildrush/setup-php/commit/89a79bb15c52542232c3342081bf56d5c4952c3c))
* add Phase 2 compat-first slice design spec ([bfdf498](https://github.com/buildrush/setup-php/commit/bfdf4985a6b2f107cbc8a00f8cb25be13454f043))
* add T12 hand-off with concrete configure-flag diff for local iteration ([422a927](https://github.com/buildrush/setup-php/commit/422a92719a8357c3962575676d5b5099a8d0c3ea))
* audit shivammathur/setup-php@v2 compatibility baseline ([c515d70](https://github.com/buildrush/setup-php/commit/c515d70c41130c3845eee9e8aa0b3aa274bd5c6c))
* **compose:** correct WriteDisableExtension doc to match codebase conventions ([8bc9a70](https://github.com/buildrush/setup-php/commit/8bc9a70d7bde3dff1d373a7013abd94ab471e13b))
* fix duplicate pdo row and non-atomic priority in compat matrix ([e1f7cee](https://github.com/buildrush/setup-php/commit/e1f7ceee20e9f4450df7cd7f135cc33064fbb929))
* **readme:** advertise drop-in compat and list v2-compat inputs ([9862122](https://github.com/buildrush/setup-php/commit/98621224e53c2aac764549c2018b1c7e822177f6))
* **readme:** hedge compiled-in baseline claim to acknowledge current delta ([1378cda](https://github.com/buildrush/setup-php/commit/1378cda6ae577d3b8b3269c8e60c58afa729e057))
* **smoke:** drop reference to non-existent BUILDRUSH_OFFLINE_DIR ([81ab529](https://github.com/buildrush/setup-php/commit/81ab529349e67ed40c3c523ea8284ec3c1800e7d))

## [1.1.2](https://github.com/buildrush/setup-php/compare/v1.1.1...v1.1.2) (2026-04-17)


### Bug Fixes

* stop treating the floating major tag as a GitHub release ([#17](https://github.com/buildrush/setup-php/issues/17)) ([f04e1e4](https://github.com/buildrush/setup-php/commit/f04e1e4d19330cc8583c0fc257cf0e35190ceb66))

## [1.1.1](https://github.com/buildrush/setup-php/compare/v1.1.0...v1.1.1) (2026-04-17)


### Bug Fixes

* build and upload release assets from release-please workflow ([#15](https://github.com/buildrush/setup-php/issues/15)) ([bd8a48c](https://github.com/buildrush/setup-php/commit/bd8a48cc507e7b20dc40a8b04c285b65f6416e74))

## [1.1.0](https://github.com/buildrush/setup-php/compare/v1.0.1...v1.1.0) (2026-04-17)


### Features

* install coverage driver when coverage input is set ([#14](https://github.com/buildrush/setup-php/issues/14)) ([e3b90cd](https://github.com/buildrush/setup-php/commit/e3b90cdbfc37b373c0a8628593cebef2d3b3f3b6))


### Bug Fixes

* move floating major tag from release-please workflow ([#10](https://github.com/buildrush/setup-php/issues/10)) ([1852579](https://github.com/buildrush/setup-php/commit/1852579f483774a3a7b5c0bf1296e4b50c106467))

## [1.0.1](https://github.com/buildrush/setup-php/compare/v1.0.0...v1.0.1) (2026-04-17)


### Bug Fixes

* drop component prefix from release-please tags ([#4](https://github.com/buildrush/setup-php/issues/4)) ([29a47c5](https://github.com/buildrush/setup-php/commit/29a47c54bcea60eac11a47cc176d92732ed25806))
* resolve release tag via GitHub API when downloading phpup ([#6](https://github.com/buildrush/setup-php/issues/6)) ([f6676ca](https://github.com/buildrush/setup-php/commit/f6676ca45470ac25d310c044ef90acf3b1b9b265))

## 1.0.0 (2026-04-17)


### Features

* add pcov, apcu, and xdebug extensions ([#2](https://github.com/buildrush/setup-php/issues/2)) ([f1f58a3](https://github.com/buildrush/setup-php/commit/f1f58a3b1a976bb341b3607d5564c295008b0a04))
* implement update-lockfile.sh to query GHCR for bundle digests ([443de0b](https://github.com/buildrush/setup-php/commit/443de0b8496642d909c6cfb1b1f56cf4821c3c16))


### Bug Fixes

* add --allow-path-traversal to oras pull (required by v1.3.x) ([b2162cc](https://github.com/buildrush/setup-php/commit/b2162ccdf481521d6a0d6e7446afe91378beaad9))
* add --disable-path-validation to oras push (required by v1.3.x) ([a52d7fc](https://github.com/buildrush/setup-php/commit/a52d7fc5b0b5ade54167d865c28cb41283b1360c))
* add verbose build logging and resolve PHP minor to patch version ([b7c884f](https://github.com/buildrush/setup-php/commit/b7c884ff84d53b331695f3c3387203fed36958fe))
* align fetch-core.sh tag format with push tag format (VERSION-OS-ARCH-TS) ([64a704c](https://github.com/buildrush/setup-php/commit/64a704c3f7de1cf3b4ce5d74d2b57521e1da60a3))
* handle absolute paths in oras pull for core bundle fetch ([ca44616](https://github.com/buildrush/setup-php/commit/ca4461623dd8ef5e640274cf129835e7dd20bd18))
* key PHP lockfile entries by catalog version (minor) not resolved patch ([54deacc](https://github.com/buildrush/setup-php/commit/54deacc65728fee5caabc2cf50b91861ea0c7b2b))
* lockfile auto-commit and watch job permissions ([cfde129](https://github.com/buildrush/setup-php/commit/cfde129093986f3bc5865dfa790f69e75e9cdac2))
* override extension_dir to match bundle location at runtime ([984ecf2](https://github.com/buildrush/setup-php/commit/984ecf263f318d21e22b9dc32bfd5ae0502d3db9))
* remove invalid --disable-path-validation from oras pull ([bd8b376](https://github.com/buildrush/setup-php/commit/bd8b37650f028bd312b1477aa75ee311115aee91))
* revert cosign-installer to v3 (no floating v4 tag exists) ([4929839](https://github.com/buildrush/setup-php/commit/4929839a9a98af67808d00b2f5c6961cbd702d85))
* run update-lock even when build jobs are skipped ([5904ad0](https://github.com/buildrush/setup-php/commit/5904ad02b9a527bc742bd348452a0f4618c81f37))
* strip sha256: prefix from digest before cosign signing ([07711bf](https://github.com/buildrush/setup-php/commit/07711bf626d58d3256eca2d3f10888e2d1b97980))
* symlink PHP prefix so phpize resolves build files correctly ([7a189ed](https://github.com/buildrush/setup-php/commit/7a189ed99d6eb248ddc214e5ae8616ae58a341ef))
* use create-pull-request for lockfile update (branch protection) ([c0a67ff](https://github.com/buildrush/setup-php/commit/c0a67ff95c5efe4b40eb34bd1ad8042d6ad51cf9))
* use layer digest instead of manifest digest in lockfile ([87a65f1](https://github.com/buildrush/setup-php/commit/87a65f136c574ed5c6acaf53937fb90fb95beb3f))
* use manifest digest for OCI content-addressing in lockfile ([b1cb5d0](https://github.com/buildrush/setup-php/commit/b1cb5d07c6f8c6e0f210122f6133c8d6b66fc728))
