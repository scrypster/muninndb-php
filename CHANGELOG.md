# Changelog

All notable changes to MuninnDB are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- Vault rename — metadata-only rename with full test coverage
- Native TLS support via `--tls-cert` and `--tls-key` flags on all client-facing servers
- OpenAPI 3.0 spec served at `GET /api/openapi.yaml` (60+ routes documented)
- API key TTL — optional `expires` field on key creation (`"90d"`, `"1y"`, RFC3339)
- Query timeout enforcement — 30s activation deadline with BFS short-circuit (configurable via `MUNINN_ACTIVATE_TIMEOUT`)
- Observability tab in Web UI with live polling
- `GET /api/admin/observability` REST endpoint
- `Engine.Observability()` full system snapshot assembler
- Per-vault per-operation ring-buffer latency tracker with percentile reporting
- Vault-labeled Prometheus histograms for write/activate/read latency
- Write/Activate/Read operations instrumented with latency tracking and Prometheus
- `vault reembed` command (CLI, REST, Web UI) to re-embed all memories in a vault using the current embedder
- CHANGELOG.md following Keep a Changelog format
- Encryption at rest documentation (LUKS, FileVault, BitLocker, Docker)
- CI OpenAPI spec validation with Redocly lint
- PR template with release checklist
- Hookify rules for API spec and SDK drift detection

### Fixed
- Windowed average calculation in latency tracker
- Unconditional Prometheus metric recording and reembed vault response handling
- MCP vault default fix

---

## [0.2.5] - 2026-02-27

### Added
- `bge-small-en-v1.5` embedder support as an alternative to the default ONNX embedder
- Recall mode presets exposed in CLI, REST, and Web UI

### Fixed
- Arrow key navigation in the `init` wizard multi-select and single-select prompts

---

## [0.2.4] - 2026-02-26

### Added
- Hebbian edge pruning — low-weight associative edges are automatically pruned over time
- Activation snapshot isolation so snapshots cannot observe mid-propagation state
- Auto-sync of the PHP SDK to the `muninndb-php` repository on tag push (CI)

### Changed
- License switched to Business Source License (BSL) 1.1
- Added provisional patent notice

---

## [0.2.3] - 2026-02-26

### Added
- Node.js and PHP SDKs alongside the existing Python SDK
- Expanded REST API surface to support new SDK operations
- Server version displayed on the login screen and sidebar in the Web UI

### Fixed
- Temporal scoring accuracy and activation precision
- Stale `dist/` artifacts that blocked PyPI publish in CI
- Test mocks and temporal test thresholds updated for correctness

### Changed
- Added Apache 2.0 license, NOTICE file, and Contributor License Agreement (CLA)

---

## [0.2.2] - 2026-02-25

### Fixed
- Dashboard CSS 404 error on first load
- CLI `init` interactive prompts not rendering correctly

---

## [0.2.1] - 2026-02-25

### Fixed
- Windows binary missing from GitHub release archive
- PyPI auto-publish not triggering on tag push (CI)

---

## [0.2.0] - 2026-02-25

### Added
- Windows support — `install.ps1`, embedded ORT DLL, daemon lifecycle, and CI pipeline
- gRPC export transport
- REST backup and restore handler
- Replication coordinator and WAL improvements
- CLI `backup` / `restore` commands and vault authentication
- MCP server guided onboarding flow and Codex support
- Cohere, Google, Jina, and Mistral embedding provider plugins
- PAS (Passive-Active-Sleep) state transitions with checkpoints and migration
- Bundled ONNX embedder is always-on with async ready notification
- Default vault is public on first run; default `root` / `password` credentials auto-provisioned
- Vault export and import as `.muninn` archives (CLI, REST, engine)

### Changed
- Production hardening across storage, engine, and transport layers
- Improved engine lifecycle logging and error handling

### Fixed
- Data race in `tailLog` tests under the `-race` detector
- Vault dispatch tests that required a running server (now properly mocked)
- Flaky integration test for the temporal filter
- Windows CI smoke test failures

### Removed
- Internal eval harnesses and setup scripts

---

## [0.1.0] - 2026-02-23

### Added
- Initial public release of MuninnDB — the cognitive database
- Core memory engine with semantic write, activate, and recall operations
- Associative graph with Hebbian-inspired edge weighting
- Novelty detection with async worker pipeline
- Bundled ONNX sentence-embedding model (no external embedding service required)
- REST API server with vault-based multi-tenancy and JWT authentication
- MCP (Model Context Protocol) server for AI agent integration
- Web UI with dashboard and vault management
- Python SDK with optional LangChain `BaseMemory` integration
- CLI (`muninn init`, `muninn start`, `muninn stop`, and related commands)
- Homebrew tap and Docker image publishing via CI
- Race-detector-clean test suite with CLI integration tests

---

## Comparison Links

[Unreleased]: https://github.com/scrypster/muninndb/compare/v0.2.5...HEAD
[0.2.5]: https://github.com/scrypster/muninndb/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/scrypster/muninndb/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/scrypster/muninndb/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/scrypster/muninndb/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/scrypster/muninndb/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/scrypster/muninndb/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/scrypster/muninndb/releases/tag/v0.1.0
