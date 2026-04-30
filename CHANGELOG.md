# Changelog

## [2.1.0](https://github.com/sergio-bershadsky/secondbrain-db/compare/v2.0.0...v2.1.0) (2026-04-30)


### Features

* embeddable Go library at pkg/sbdb ([#41](https://github.com/sergio-bershadsky/secondbrain-db/issues/41)) ([7955bff](https://github.com/sergio-bershadsky/secondbrain-db/commit/7955bff9fe77a3950fc0d540d296bba73826b599))

## [2.0.0](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.3.1...v2.0.0) (2026-04-29)


### ⚠ BREAKING CHANGES

* data/<entity>/records.yaml and .integrity.yaml are no longer written or read on the active code path. Existing v1 KBs must run `sbdb doctor migrate` once before using v2.

### Features

* drop data/ layer; per-md sidecars; remove builtin templates ([#37](https://github.com/sergio-bershadsky/secondbrain-db/issues/37)) ([891bc00](https://github.com/sergio-bershadsky/secondbrain-db/commit/891bc000099e4482deea174f2501c90ec9d2c730))

## [1.3.1](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.3.0...v1.3.1) (2026-04-28)


### Bug Fixes

* **version:** resolve via debug.BuildInfo so go-install builds report the right version ([#25](https://github.com/sergio-bershadsky/secondbrain-db/issues/25)) ([8ab59e4](https://github.com/sergio-bershadsky/secondbrain-db/commit/8ab59e46c28744b8fecfb42f96bbffb2e5f4f923)), closes [#24](https://github.com/sergio-bershadsky/secondbrain-db/issues/24)

## [1.3.0](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.2.3...v1.3.0) (2026-04-28)


### Features

* **cli:** support \`sbdb --version\` flag ([#22](https://github.com/sergio-bershadsky/secondbrain-db/issues/22)) ([cfbbf34](https://github.com/sergio-bershadsky/secondbrain-db/commit/cfbbf3453273edb40b7b7423da3e768111d21858)), closes [#21](https://github.com/sergio-bershadsky/secondbrain-db/issues/21)

## [1.2.3](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.2.2...v1.2.3) (2026-04-28)


### Miscellaneous Chores

* release as 1.2.3 ([a4d0b46](https://github.com/sergio-bershadsky/secondbrain-db/commit/a4d0b468e6c016bf3c9c94586a23a9900b5f0678))

## [1.2.2](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.2.1...v1.2.2) (2026-04-28)


### Miscellaneous Chores

* release as 1.2.2 ([87f481e](https://github.com/sergio-bershadsky/secondbrain-db/commit/87f481e7bd5758c2ecfb62df8cb2d630a22c845f))

## [1.2.1](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.2.0...v1.2.1) (2026-04-28)


### Miscellaneous Chores

* release as 1.2.1 ([0529357](https://github.com/sergio-bershadsky/secondbrain-db/commit/0529357214428230fd60bcc185fe8e9b7c9d5b94))

## [1.2.0](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.1.1...v1.2.0) (2026-04-27)


### ⚠ BREAKING CHANGES

* **events:** events are no longer stored on disk. Pre-existing .sbdb/events/ directories are orphaned and may be removed. Workers that tailed .sbdb/events/*.jsonl switch to piping sbdb events emit. The [events] config section is silently ignored. The CLI surface changes: sbdb event {append,show,types,repair,rebuild-registry} are removed; only sbdb events emit remains.

### Features

* **events:** project events from git history; remove on-disk events log ([#15](https://github.com/sergio-bershadsky/secondbrain-db/issues/15)) ([382609e](https://github.com/sergio-bershadsky/secondbrain-db/commit/382609eeccf0bfc397a4450f9130671664f5f134)), closes [#14](https://github.com/sergio-bershadsky/secondbrain-db/issues/14)


### Bug Fixes

* **ci:** bump Go toolchain to 1.25.3 to clear govulncheck stdlib advisories ([#12](https://github.com/sergio-bershadsky/secondbrain-db/issues/12)) ([f8f0fa7](https://github.com/sergio-bershadsky/secondbrain-db/commit/f8f0fa732c0c981110ee9dda173299a891b33200)), closes [#11](https://github.com/sergio-bershadsky/secondbrain-db/issues/11)


### Miscellaneous Chores

* release as 1.2.0 ([87be375](https://github.com/sergio-bershadsky/secondbrain-db/commit/87be375b38d8a36b67a54964d6062bd1f3295557))

## [1.1.1](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.1.0...v1.1.1) (2026-04-27)


### Miscellaneous Chores

* release 1.1.1 ([c8c338e](https://github.com/sergio-bershadsky/secondbrain-db/commit/c8c338ea54b7d32d53a6bf3c15e328829f7b2bf7))

## [1.1.0](https://github.com/sergio-bershadsky/secondbrain-db/compare/v1.0.0...v1.1.0) (2026-04-26)


### Features

* **events:** implement append-only event log with archival ([1d65ede](https://github.com/sergio-bershadsky/secondbrain-db/commit/1d65ede5c0c33425e5be009d0134e4efbb04a462))

## 1.0.0 (2026-04-13)


### Features

* add adr, discussion, task schemas + init templates ([94d62db](https://github.com/sergio-bershadsky/secondbrain-db/commit/94d62dbfc4d3bd7ab90c60cf9bfc1265707f85da))
* add reusable doctor GitHub Action + interactive init wizard ([59969f4](https://github.com/sergio-bershadsky/secondbrain-db/commit/59969f40fa3e484ed57c05021d42d5e40c0a219e))
* add two-tier file tracking — untracked-but-signed files + bbolt graph store ([c74d5d6](https://github.com/sergio-bershadsky/secondbrain-db/commit/c74d5d60dcc1b7051c742f9d6a7ab1970f66b00b))
* initial implementation of secondbrain-db ([ac856eb](https://github.com/sergio-bershadsky/secondbrain-db/commit/ac856eb864ce77516e339a680e640c1fee96069b))


### Bug Fixes

* **ci:** minimal golangci config for v2.11 compat ([a3e6a5d](https://github.com/sergio-bershadsky/secondbrain-db/commit/a3e6a5daecd52c70c16cb7edc0a1efac3579da3f))
* **ci:** simplify golangci config, disable coverage badge ([bb0ec88](https://github.com/sergio-bershadsky/secondbrain-db/commit/bb0ec88ce32f67cbf987011bedd71a88655047aa))
* **ci:** use golangci-lint latest, exclude semantic/version from coverage ([e34c253](https://github.com/sergio-bershadsky/secondbrain-db/commit/e34c25326c0179b2f4b23c790837f79276fe978b))
* **ci:** use golangci-lint-action v7 for lint v2, enable CGO for tests ([6ed3b61](https://github.com/sergio-bershadsky/secondbrain-db/commit/6ed3b61c377b8a4179a4212955b8b2834de60b68))
* cross-platform test fixes for Windows CI ([86b6493](https://github.com/sergio-bershadsky/secondbrain-db/commit/86b6493ec69a48d0a14a2d74d747ed51eee961d2))
* **lint:** use fmt.Fprintf instead of WriteString(Sprintf) ([ea73215](https://github.com/sergio-bershadsky/secondbrain-db/commit/ea7321520f376ac3f7ad141216b894a81c532ab5))
* make notes and tasks schemas monthly-partitioned ([18eee26](https://github.com/sergio-bershadsky/secondbrain-db/commit/18eee261f942f8cea013a39a968e53d16761445a))
