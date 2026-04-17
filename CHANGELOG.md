# Changelog

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
