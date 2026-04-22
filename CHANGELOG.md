# Changelog

## [1.7.0](https://github.com/buildrush/setup-php/compare/v1.6.0...v1.7.0) (2026-04-22)


### Features

* **builders/linux:** parametrize build-ext.sh arch ([ff1b597](https://github.com/buildrush/setup-php/commit/ff1b597c9bfbb91b1164530d89290fbafc84ba14))
* **catalog:** enable aarch64 builds on all 5 PHP cores ([5189111](https://github.com/buildrush/setup-php/commit/5189111c5b8af1a3cd190941d517090a024d9277))
* **compat:** per-arch DefaultIniValues (aarch64 jit_buffer_size=128M) ([e7a99ec](https://github.com/buildrush/setup-php/commit/e7a99ec7f935e9f67a8a64c4c4528ac0e6980390))
* easy+medium PECL extensions on aarch64 (phase 2 C2a) ([#48](https://github.com/buildrush/setup-php/issues/48)) ([276d89c](https://github.com/buildrush/setup-php/commit/276d89c1ebf3cc01f99b976394fa7859ed468bfd))
* hard-tier PECL extensions on aarch64 (phase 2 C2b) ([#49](https://github.com/buildrush/setup-php/issues/49)) ([d3ce025](https://github.com/buildrush/setup-php/commit/d3ce02538f279d57e0afb03512aad9e42db754f8))
* **workflows:** runs-on mapping for aarch64 matrix cells ([e8062a6](https://github.com/buildrush/setup-php/commit/e8062a669e1071a2949e38c617dbe6f063b767d0))


### Bug Fixes

* **workflows:** build phpup for both amd64 and arm64 in compat-harness ([0bb7e45](https://github.com/buildrush/setup-php/commit/0bb7e458cf67bedd29fd7c164bd1eecace7f1c8d))
* **workflows:** harness fixture filter is (version, arch)-aware ([5a0da10](https://github.com/buildrush/setup-php/commit/5a0da10b521c2d2cc1e20b4fe674301014f3a678))
* **workflows:** per-arch oras download in build-php-core / build-extension ([713c140](https://github.com/buildrush/setup-php/commit/713c1400937f6a4b86ce715c031bf400734d9b2b))


### Documentation

* add Phase 2 aarch64 C2a design (easy+medium PECL extensions) ([bea0345](https://github.com/buildrush/setup-php/commit/bea0345ae34585b7e93a54a188900200f5e208b1))
* add Phase 2 aarch64 C2b design (hard-tier PECL on arm64) ([3913064](https://github.com/buildrush/setup-php/commit/3913064f3bf73980c545b7a48236166f38725569))
* add Phase 2 slice D design (ubuntu-22.04 runner coverage) ([26c0131](https://github.com/buildrush/setup-php/commit/26c01312b922cacbc8c0dff908e9b36cbfaa448e))
* **compat-matrix:** record audited jit_aarch64.ini ([ef3fd3f](https://github.com/buildrush/setup-php/commit/ef3fd3ffbbb7256c06b89731c875f5a954b8b04f))

## [1.6.0](https://github.com/buildrush/setup-php/compare/v1.5.0...v1.6.0) (2026-04-21)


### Features

* **builders/linux:** install build_deps from catalog via workflow ([ca9f41e](https://github.com/buildrush/setup-php/commit/ca9f41e44dffe9f0b88931c1450f9fb1e04318f6))
* **catalog:** add build_deps field to ExtensionSpec ([fff6a3a](https://github.com/buildrush/setup-php/commit/fff6a3a63e64a49056bafdf37e3994b177fd9346))
* **catalog:** add hard-tier PECL extensions (phase 2 B2) ([035fb11](https://github.com/buildrush/setup-php/commit/035fb11b2aae06eaaaa9c500dc5a96cc2f28585f))
* **catalog:** add top-10 PECL extensions (phase 2 B1) ([e7bf8b4](https://github.com/buildrush/setup-php/commit/e7bf8b4702e3b6fba07ad84a8df555dacae89d30))
* **catalog:** enable PHP 8.1 core build (linux/x86_64/nts) ([8ec7c18](https://github.com/buildrush/setup-php/commit/8ec7c1882377b19595871be4054d2d322362b788))
* **catalog:** enable PHP 8.2 core build (linux/x86_64/nts) ([849724e](https://github.com/buildrush/setup-php/commit/849724e10f2111bc4f09706c78731a8415490e30))
* **catalog:** enable PHP 8.3 core build (linux/x86_64/nts) ([2b87c9c](https://github.com/buildrush/setup-php/commit/2b87c9c8ff13c4ee7b6679ad6b188ab400e831a5))
* **catalog:** extend PECL extensions to PHP 8.1 ABI ([38f2a01](https://github.com/buildrush/setup-php/commit/38f2a01b852cad2562937b0b43785e1a76b42eea))
* **catalog:** extend PECL extensions to PHP 8.2 ABI ([54037ff](https://github.com/buildrush/setup-php/commit/54037ff979507aa690a80531a9a84f77c8e109c0))
* **catalog:** extend PECL extensions to PHP 8.3 ABI ([08ac39e](https://github.com/buildrush/setup-php/commit/08ac39e4d3405e6104d7edf6fa406e522b863282))
* enable PHP 8.5 core + PECL rebuild (phase 2) ([#37](https://github.com/buildrush/setup-php/issues/37)) ([71589ef](https://github.com/buildrush/setup-php/commit/71589ef6a432f31eee8ef06b92b6fbea8cfeeeba))
* **phpup:** extend runtime catalog with hard-tier PECL extensions ([6bfbb30](https://github.com/buildrush/setup-php/commit/6bfbb304a32d9d344deeef76b19e95d9b6957486))
* **phpup:** extend runtime catalog with top-10 PECL extensions ([0f3cf55](https://github.com/buildrush/setup-php/commit/0f3cf55b0f29184ba87821b2eede1599fec46ba7))
* **phpup:** install runtime_deps.linux for PECL bundles ([e844105](https://github.com/buildrush/setup-php/commit/e844105ceb43f1fb961953d138caf310396f6b2b))
* **workflows:** descriptive job names for build-extension / build-php-core ([#45](https://github.com/buildrush/setup-php/issues/45)) ([ea659f0](https://github.com/buildrush/setup-php/commit/ea659f05816ab7f17634777e247407cbf4334054))


### Bug Fixes

* **catalog:** exclude igbinary on 8.5; swap affected fixture ([2656f27](https://github.com/buildrush/setup-php/commit/2656f2731121846443e335a84cfa89c016441a7c))
* **catalog:** imagick runtime_deps package names on ubuntu-24.04 ([8fac81b](https://github.com/buildrush/setup-php/commit/8fac81bfb5e37608c380d90a1020cfeec9bf2443))


### Documentation

* add Phase 2 aarch64 C1 design (infrastructure + cores) ([66a0604](https://github.com/buildrush/setup-php/commit/66a0604f0c38ce7f2cddd99f437806b4fc297fc3))
* add Phase 2 hard-tier PECL extension design (slice B2) ([1be8736](https://github.com/buildrush/setup-php/commit/1be87366df631835bb1cc8538a978f36c0bd125c))
* add Phase 2 top-10 PECL extension expansion design (slice B1) ([da4f6f9](https://github.com/buildrush/setup-php/commit/da4f6f9b11a87fab854245f876d42d2b3b0ace68))
* add Phase 2 version-expansion design ([5779ca3](https://github.com/buildrush/setup-php/commit/5779ca3f119f964a720e157586a80f4ee0074420))
* amend phase2-version-expansion — real fixture count (8), existing compat tests ([d823a5a](https://github.com/buildrush/setup-php/commit/d823a5a58e742eb0b37252c5a4dc1cd3fb26cb8a))
* **readme:** list PHP 8.1–8.5 as supported after phase 2 version expansion ([69af03f](https://github.com/buildrush/setup-php/commit/69af03f216493bb7727a32fafe40fd7a50a465bd))

## [1.5.0](https://github.com/buildrush/setup-php/compare/v1.4.0...v1.5.0) (2026-04-21)


### Features

* **builders/linux:** stash php.ini-{production,development} into bundle ([e3b3289](https://github.com/buildrush/setup-php/commit/e3b328924de502d4f5dc7b8d64704e5a5501b0e6))
* bundle rollout PR-α — commit-back lockfile flow ([935bc93](https://github.com/buildrush/setup-php/commit/935bc936f3090d531af13f946343bbf99a4da3bf))
* bundle rollout PR-β.1 — sidecar schema_version + runtime enforcement ([eb8d6e1](https://github.com/buildrush/setup-php/commit/eb8d6e1a3b841ff797402de19d7ced62eb417b30))
* bundle rollout PR-β.2 — GC + release-lockfile invariant ([c212a86](https://github.com/buildrush/setup-php/commit/c212a8668a2e88e7eca0c77a1448cfee4341f65b))
* **compat:** add BaseIniFileName for ini-file input ([ee3e604](https://github.com/buildrush/setup-php/commit/ee3e604b1391f52a3a42593b5d80653d112418cb))
* **compat:** add PHP 8.x opcache/JIT defaults per v2 jit.ini ([c13504a](https://github.com/buildrush/setup-php/commit/c13504a4b109d7e17b6557aa4c0949139d9b5636))
* **compat:** add XdebugIniFragment matching v2's xdebug.ini ([cfe5e60](https://github.com/buildrush/setup-php/commit/cfe5e60e27d470a25f0cbcd502f8b002148873ea))
* **compose:** MergeCompatLayers helper for ini precedence ([2304ecd](https://github.com/buildrush/setup-php/commit/2304ecd7517fabcbac50bb696107bbcc41d67587))
* **compose:** SelectBaseIniFile — copy production/development or empty ([1c15f05](https://github.com/buildrush/setup-php/commit/1c15f05401bd43a34383ea68a8b37af530dfa505))
* **phpup:** wire ini-file selection + XdebugIniFragment + ExtraIni ([988456a](https://github.com/buildrush/setup-php/commit/988456ab5cac9ad170b28b2b7a6aa092a34ddb9e))
* **plan:** coverage side-effects — disable unused driver, pcov.enabled=1 ([5ae90d0](https://github.com/buildrush/setup-php/commit/5ae90d0e1ce22ceb0da6bb059f0b8601d5183a2e))


### Bug Fixes

* **cmd/lockfile-update:** preserve generated_at on no-op bundle updates ([c8151c9](https://github.com/buildrush/setup-php/commit/c8151c90d7d48a3a932db6e280a5b8bc7a9ebf4a))
* **phpup,compat:** honor opcache exclusion; narrow xdebug fragment; allowlist residual drift ([93d30cd](https://github.com/buildrush/setup-php/commit/93d30cd1f434e6dabb935f10c56efb89cf3624ec))
* **phpup:** set PHPRC and auto-load opcache ([41f7dc5](https://github.com/buildrush/setup-php/commit/41f7dc53ad533249a3d62ccb3f84fc053a96b404))
* **plan:** include IniFile in Hash() to prevent cache contamination ([800a234](https://github.com/buildrush/setup-php/commit/800a23402d4b9965b496e2acd49f8e011116799b))
* **planner:** use phpMinor (not PHPAbi) for ext lockfile key lookup ([c8a1903](https://github.com/buildrush/setup-php/commit/c8a19031a0c1a48167ff82ce4e296a3245dc5305))
* **planner:** use tag-resolve instead of digest-HEAD for skip check ([10097f3](https://github.com/buildrush/setup-php/commit/10097f356ae58afe28c78cc0603e007cbf4e4be4))
* **plan:** validate coverage input; reject invalid values with ::error:: ([b7b7901](https://github.com/buildrush/setup-php/commit/b7b790147288229383c4bb79b4ff549d4fe7727d))
* **workflows:** plan.yml checks out PR head, not merge commit ([888ece2](https://github.com/buildrush/setup-php/commit/888ece25e211a25067f3ea4add7a2f8854c7223b))


### Documentation

* add Phase 2 compat closeout design ([0be4974](https://github.com/buildrush/setup-php/commit/0be49744117e66bd25aef74e5c0357aa41a0c094))
* close bundle-rollout spec Tβ-9 doc gaps ([09957be](https://github.com/buildrush/setup-php/commit/09957bef556164dd311956331cfb65bdb4cc7ab6))
* **compat:** clarify XdebugIniFragment divergence + add 7.3 test ([00add22](https://github.com/buildrush/setup-php/commit/00add22a9225ba76e5e61eda52e0ed59607da3a6))
* **compat:** mark coverage+ini-file quirks as implemented ([9e59f50](https://github.com/buildrush/setup-php/commit/9e59f507a08d1021bb8b7bd4190b5bd7a3150f60))
* **compat:** shrink deviations allowlist for closeout slice ([7394aa1](https://github.com/buildrush/setup-php/commit/7394aa14c524341ea3ebe46078ac685ee5eadc92))

## [1.4.0](https://github.com/buildrush/setup-php/compare/v1.3.0...v1.4.0) (2026-04-20)


### Features

* **compat-diff:** diff flattened probes with allowlist kinds ([b0d3e4f](https://github.com/buildrush/setup-php/commit/b0d3e4f36a38b568e4a2f619e5a9c11ed9d7b7f6))
* **compat-diff:** load deviations allowlist from compat-matrix.md ([2fa84f4](https://github.com/buildrush/setup-php/commit/2fa84f472bfd9cd2d27d83b38708dfe5800504be))
* **compat-diff:** scaffold CLI with flag parsing ([e6badf9](https://github.com/buildrush/setup-php/commit/e6badf94a5c68de9eff2f902e31d23a4eca4fd9a))
* **compat-diff:** validate --fixture name shape ([cce9471](https://github.com/buildrush/setup-php/commit/cce9471c8bc0c99303209641e7524ecaa907aa70))
* **compat-diff:** wire end-to-end CLI with GitHub Actions annotations ([85f76a3](https://github.com/buildrush/setup-php/commit/85f76a3064d622e833f2940703247022edf2ac67))
* **compat:** add 6-fixture compat harness manifest ([6ddffd3](https://github.com/buildrush/setup-php/commit/6ddffd3ac26f01fd4711d17c9dd3003de21977ea))
* **compat:** curate ini keys recorded by probe ([94e3528](https://github.com/buildrush/setup-php/commit/94e35289202d02ed74673977209517b6196b96d1))
* **compat:** probe computes env_delta and normalized path_additions ([71e6d9c](https://github.com/buildrush/setup-php/commit/71e6d9c4a2483fcc401534baab479303b72e11d2))
* **compat:** probe script for php version/ext/ini fields ([9f87197](https://github.com/buildrush/setup-php/commit/9f8719729727fc0d0eea121774f48275f3cf0d68))
* **workflows:** compat-harness 'ours' matrix job ([3d45da4](https://github.com/buildrush/setup-php/commit/3d45da465f6cae60a4f7207b9ec3481954ab9200))
* **workflows:** compat-harness 'theirs' matrix job pinned to v2 SHA ([39cfae6](https://github.com/buildrush/setup-php/commit/39cfae687e5cb6eea55f0eac69ba40c7aa7372e9))
* **workflows:** compat-harness diff job and compat-gate ([6db35a3](https://github.com/buildrush/setup-php/commit/6db35a39b4cb794fb72c58548b29d90e0f3b8604))
* **workflows:** compat-harness skeleton (build + fixtures) ([57ccf03](https://github.com/buildrush/setup-php/commit/57ccf03ce1b74dfa9f98e2d87d778dcbe621ad52))


### Bug Fixes

* **compat-diff:** satisfy errcheck + gocritic lints ([da2001a](https://github.com/buildrush/setup-php/commit/da2001ab2c5c11aa3f2591f802f8017f8e0f392a))
* **compat:** probe escapes newlines/tabs/CRs in ini values ([153a7db](https://github.com/buildrush/setup-php/commit/153a7dbd83339a2494b9b45c6e283f052159f138))
* **workflows:** grant write permissions on pipeline job in each wrapper ([20b0786](https://github.com/buildrush/setup-php/commit/20b0786cf56c0739380dac7a712ede79ae7f0baa))
* **workflows:** use yq+jq pipeline for fixture matrix generation ([f091dc2](https://github.com/buildrush/setup-php/commit/f091dc24758617afa6277a64c3f040266ad096f9))


### Documentation

* add compat-harness design spec ([a875d71](https://github.com/buildrush/setup-php/commit/a875d7105cc9122795d3612284f9e1dd1f1f4660))
* **compat:** add deviations allowlist markers ([8ac97af](https://github.com/buildrush/setup-php/commit/8ac97af5b2315445e8a0e40e8dd752022e77738f))
* **compat:** allowlist Phase-2-in-progress deviations ([b7cddd1](https://github.com/buildrush/setup-php/commit/b7cddd1587688adcd484462c89349653b45d2055))
* **readme:** link to compat-harness workflow and allowlist ([1b93262](https://github.com/buildrush/setup-php/commit/1b93262a99767714c7f65e9561ac40ab9f0073e0))

## [1.3.0](https://github.com/buildrush/setup-php/compare/v1.2.0...v1.3.0) (2026-04-17)


### Features

* **workflows:** add plan-and-build reusable pipeline ([799a66c](https://github.com/buildrush/setup-php/commit/799a66c9eb9ee7af1f90fce53d99c35c43723a4c))
* **workflows:** wire manual dispatch through plan-and-build ([5cd5775](https://github.com/buildrush/setup-php/commit/5cd5775843d27dd8082eedbf7a002310b1937090))
* **workflows:** wire nightly schedule through plan-and-build ([cb22e5a](https://github.com/buildrush/setup-php/commit/cb22e5ad6570c01af3305ec8ac83f98877281646))
* **workflows:** wire security-rebuild dispatch through plan-and-build ([3695d3a](https://github.com/buildrush/setup-php/commit/3695d3a6c062cbfd8b6dedb276dba831e53b0593))


### Documentation

* add plan-and-build reusable workflow design spec ([765aaf8](https://github.com/buildrush/setup-php/commit/765aaf8bd3d22a6df7ab559fee37c7bb86922d66))

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
