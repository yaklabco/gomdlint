# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.2](https://github.com/jamesainslie/mdlint/compare/v0.1.1...v0.1.2) (2025-12-08)

### Continuous Integration

- trigger release workflow on tag push ([c0c94a0](https://github.com/jamesainslie/mdlint/commit/c0c94a02664e288d5a5d07d42233cea1b841cfad))
- trigger release-please on push to main ([911d769](https://github.com/jamesainslie/mdlint/commit/911d7696c826219a845698c874c2d1be33f160e0))

## [0.1.1](https://github.com/jamesainslie/mdlint/compare/gomdlint-v0.1.0...gomdlint-v0.1.1) (2025-12-08)

### Features

- add cigate target to stavefile for CI checks ([610aab2](https://github.com/jamesainslie/mdlint/commit/610aab2080e157c065166263ea82adb76c1043f1))
- **ast:** add reference styles and fence metadata ([d7743ed](https://github.com/jamesainslie/mdlint/commit/d7743edd013b5d6e075407219515d9edd765e6e0))
- **cli:** add init and migrate commands with config integration ([b757909](https://github.com/jamesainslie/mdlint/commit/b757909dccf9f3fa445419748be9acc47c755847))
- **cli:** implement CLI skeleton with commands ([167a9af](https://github.com/jamesainslie/mdlint/commit/167a9af1b1a99801e6bdf965b2664df636b4b0b5))
- **cli:** integrate reporters, color, and structured logging ([abcdd37](https://github.com/jamesainslie/mdlint/commit/abcdd37c3f275992246541de09f06cdf3d3cba0a))
- **cli:** wire up safety pipeline and runner in lint command ([aec1983](https://github.com/jamesainslie/mdlint/commit/aec198301bb9f1591b4f4baa3d73e7586f2bbebe))
- **configloader:** add configuration system foundation ([f589079](https://github.com/jamesainslie/mdlint/commit/f5890797558997e684d1c7c12d516a956dc53396))
- **configloader:** add markdownlint migration support ([251b465](https://github.com/jamesainslie/mdlint/commit/251b465bfc23ee8b819f150e48106a88d95c1e74))
- **fix:** add diff generation for dry-run mode ([c0636a2](https://github.com/jamesainslie/mdlint/commit/c0636a24c58acef5ca8d4eb8f6c7386b45258b68))
- **fix:** add edit validation and application ([5269ab8](https://github.com/jamesainslie/mdlint/commit/5269ab839096547a433ecbd6d5e05f9c3f06a95e))
- **fsutil:** add file system utilities for safety pipeline ([e08bb55](https://github.com/jamesainslie/mdlint/commit/e08bb55739716699d175820ac11450018849290a))
- **lint/rules:** add core lint rules for Phase 4 ([08e47c9](https://github.com/jamesainslie/mdlint/commit/08e47c9874bb74907fdab77fdcceeb985da90209))
- **lint:** add rule context, diagnostic builder, and helpers ([2aebbec](https://github.com/jamesainslie/mdlint/commit/2aebbec6257702e465a190c631c7a18e9ad497dd))
- **lint:** add rule resolution and engine ([76a4434](https://github.com/jamesainslie/mdlint/commit/76a443439da933697fced3a3de282e65bb555f76))
- **lint:** add safety pipeline for file processing ([f006317](https://github.com/jamesainslie/mdlint/commit/f006317e20f9c9416cb1d246d60da2371583c9e0))
- **mdast:** implement core Markdown AST types ([a8ccee1](https://github.com/jamesainslie/mdlint/commit/a8ccee1690b562326d2264cefa61005f916b05e2))
- **refs:** add reference tracking context ([8deb05e](https://github.com/jamesainslie/mdlint/commit/8deb05e3c69e1f66d4e46a045511b60c5938334a))
- **reporter:** add styled reporters and diff metadata ([fe6cc3a](https://github.com/jamesainslie/mdlint/commit/fe6cc3a11596120883fdfb725314041de556cf89))
- **rules:** implement extended markdownlint rules and packs ([c2ccf6d](https://github.com/jamesainslie/mdlint/commit/c2ccf6d1bb06b6fcd6dbde6940965b4b4f14ad08))
- **rules:** wire up rule registration in CLI ([47d3f34](https://github.com/jamesainslie/mdlint/commit/47d3f341bde9a70e797a20c9ada5abe0f7949b5f))
- **runner:** add file discovery with ignore pattern support ([fc49ae0](https://github.com/jamesainslie/mdlint/commit/fc49ae010b48fbe0aea78377414074f94507e777))
- **runner:** add options and result types for multi-file linting ([7ee3ca9](https://github.com/jamesainslie/mdlint/commit/7ee3ca9ffedc87b70c9f8748e9e90b531ae8c0ec))
- **runner:** implement concurrent worker pool runner ([77c4055](https://github.com/jamesainslie/mdlint/commit/77c4055f4e16dc039238feef066b1ab51b8fdb5d))

### Bug Fixes

- **ci:** add git config for private module authentication ([011b4ca](https://github.com/jamesainslie/mdlint/commit/011b4caff1f6366672ce8bfa9cbfc99ab0bff5a1))
- **ci:** remove private stave from Brewfile ([f0d10e0](https://github.com/jamesainslie/mdlint/commit/f0d10e0736d2f7b237c6dc52d4afffe7efbc730e))
- **ci:** use stave ci-clean-git-status-check branch and remove init step ([6c7ba52](https://github.com/jamesainslie/mdlint/commit/6c7ba524a1c921c15e9c76c454977476412b1955))
- correct gitignore to only ignore binary in root directory ([e2ae378](https://github.com/jamesainslie/mdlint/commit/e2ae378a91077dccad8f492cfdd0a61ee7734665))
- **fsutil:** classify file errors with sentinel types ([db9f70e](https://github.com/jamesainslie/mdlint/commit/db9f70ee0889ee7c1a3177c75480ecfd1a612f5a))
- handle missing test files in test workflow ([1933689](https://github.com/jamesainslie/mdlint/commit/193368967e5536d2f69ace5cad8d3695c16fe2a5))
- **lint/rules:** correct break condition in list number parsing ([0d0958f](https://github.com/jamesainslie/mdlint/commit/0d0958f5b2512c019f334d3b052715ead0674757))
- resolve all golangci-lint issues ([a6f0b44](https://github.com/jamesainslie/mdlint/commit/a6f0b443b857aceaea415ae2f7df39594ef370b8))
- resolve forbidigo linter errors in main.go ([1ffba24](https://github.com/jamesainslie/mdlint/commit/1ffba24bb54e9989bb98353732174cd5acc0c0a6))
- set initial version to 0.1.0 in release-please config ([c183b57](https://github.com/jamesainslie/mdlint/commit/c183b57782f5492a159470880300f6f50592a2de))

### Code Refactoring

- rename project from mdlint to gomdlint ([97583e5](https://github.com/jamesainslie/mdlint/commit/97583e5d0c27c7197a4a1383cd58aa79719f4e75))

### Tests

- add tests for CLI, logging, and mdast packages ([8ffa2ff](https://github.com/jamesainslie/mdlint/commit/8ffa2ff3a5b6eb9dbc7afde6581f5d6e9caa6929))

### Build System

- add MIT license ([0dfac3d](https://github.com/jamesainslie/mdlint/commit/0dfac3da2408d8f8f1f96a2f8e0c0ded57db1f2d))

### Continuous Integration

- add goreleaser and release-please configuration ([2bd14b4](https://github.com/jamesainslie/mdlint/commit/2bd14b46b05048d331afe85836e3393fd513a9f9))
- add lint and test workflows ([92f074e](https://github.com/jamesainslie/mdlint/commit/92f074ec531ee69678852bd256226cae22b5f178))
- configure authentication for private Go modules ([4020e04](https://github.com/jamesainslie/mdlint/commit/4020e0488a9e50b415b1d1140630316cffb6c41c))
- update Go version to 1.25 ([5946b6a](https://github.com/jamesainslie/mdlint/commit/5946b6af5a30d36d5dfd80a27a514718f4f01f6a))

## 0.1.0 (2025-11-30)

### Features

- add cigate target to stavefile for CI checks ([610aab2](https://github.com/yaklabco/gomdlint/commit/610aab2080e157c065166263ea82adb76c1043f1))
- **cli:** implement CLI skeleton with commands ([4162a74](https://github.com/yaklabco/gomdlint/commit/4162a74d8285c3155436d963809e748935dfdd6e))
- **mdast:** implement core Markdown AST types ([46aec5e](https://github.com/yaklabco/gomdlint/commit/46aec5eeb156276c7b4878c58583e64369828cec))

### Bug Fixes

- correct gitignore to only ignore binary in root directory ([e2ae378](https://github.com/yaklabco/gomdlint/commit/e2ae378a91077dccad8f492cfdd0a61ee7734665))
- handle missing test files in test workflow ([1933689](https://github.com/yaklabco/gomdlint/commit/193368967e5536d2f69ace5cad8d3695c16fe2a5))
- resolve all golangci-lint issues ([7f4303d](https://github.com/yaklabco/gomdlint/commit/7f4303d0c7d750409cb489059b90e212b1f3e53a))
- resolve forbidigo linter errors in main.go ([1ffba24](https://github.com/yaklabco/gomdlint/commit/1ffba24bb54e9989bb98353732174cd5acc0c0a6))
- set initial version to 0.1.0 in release-please config ([c183b57](https://github.com/yaklabco/gomdlint/commit/c183b57782f5492a159470880300f6f50592a2de))

### Tests

- add tests for CLI, logging, and mdast packages ([7512384](https://github.com/yaklabco/gomdlint/commit/751238471668cdb84090935772d8a7899966551a))

### Build System

- add MIT license ([0dfac3d](https://github.com/yaklabco/gomdlint/commit/0dfac3da2408d8f8f1f96a2f8e0c0ded57db1f2d))

### Continuous Integration

- add goreleaser and release-please configuration ([2bd14b4](https://github.com/yaklabco/gomdlint/commit/2bd14b46b05048d331afe85836e3393fd513a9f9))
- add lint and test workflows ([92f074e](https://github.com/yaklabco/gomdlint/commit/92f074ec531ee69678852bd256226cae22b5f178))
- update Go version to 1.25 ([351a147](https://github.com/yaklabco/gomdlint/commit/351a1470adfc9a0b16cb6eed37a62eaf5ff5d958))
