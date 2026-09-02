# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.14]

### Security

- Agent correlation mapping files moved out of the shared `/tmp` into a
  per-user, owner-only directory (`INTERMUX_MAPPING_DIR`, default
  `~/.local/state/intermux/mappings`, created `0700` with `0600` files). The
  loader refuses a directory that is group- or world-accessible and skips
  anything that isn't a regular file.

### Fixed

- Status detection no longer misreads a finished status banner earlier in
  the trailing scan window (e.g. a completed `Running 12 tests` line) as
  still-active after the shell has returned to an idle prompt. Only the
  pane's single most recent line is now checked, against a regex anchored
  to the start of the line.
- `server_info`, an already-shipped MCP tool, was undocumented; README and
  AGENTS.md said 7 tools and omitted it. Both now list all 8.

### Added

- `docs/status-detection.md`: how active/idle/stuck/crashed status is
  derived, including the false positive fixed above.
- `docs/install.md`: a client-agnostic `go install` + raw MCP config path
  that doesn't require the Claude Code plugin system.
- CONTRIBUTING.md, SECURITY.md, CODE_OF_CONDUCT.md, and issue/PR templates.

### Changed

- CI now runs on an ubuntu+macos matrix with a `gofmt` gate and the
  structural test suite, alongside the existing fleet version-skew sweep.
- Structural tests (`tests/structural/`) no longer import a sibling
  `_shared` package from outside the repo; the two fixture helpers and the
  base test class they used are vendored in `tests/structural/_vendored.py`,
  so the suite runs from a clean clone.

### Removed

- Internal planning artifacts not meant for a public reader: `PHILOSOPHY.md`,
  `docs/roadmap.md`, `docs/intermux-roadmap.md`, `docs/roadmap.json`.
- Dead code: `ParsedContent.HasErrors` (computed, never read),
  `Monitor.OnStatusChange` (declared and invoked, never assigned),
  `LoadKeywordsFromRegistry` and its registry-parsing helpers (exercised
  only by their own test), and the now-unused `gopkg.in/yaml.v3` dependency.
