# Changelog

All notable changes to DewDrops will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.3.1] - 2026-04-10

### Added

- **`--map` extension filtering**: `--map` now accepts an optional value to control which files appear in the map output. `--map` (no value) includes only files with supported signature patterns. `--map=go,py` includes only specific extensions. `--map=any` includes all text files (previous default behavior).
- **`--version` flag**: Prints the current version. Version injected via goreleaser ldflags at release time; local builds show `dev`.
- **PHP signature test**: Verifies indented class methods are captured.
- 3 new tests for map extension filtering, 1 new test for PHP signatures.

### Fixed

- **PHP, Java/Kotlin, Ruby, JS/TS, Rust signature extraction**: Changed regex patterns from `^` (column 0 only) to `^\s*` so indented class/impl methods are captured. Previously only top-level class declarations matched; all methods inside classes were missed.

### Changed

- **Multi-file layout**: Refactored from single `main.go` (~1,100 lines) into 7 focused files: `main.go`, `run.go`, `git.go`, `tree.go`, `signatures.go`, `writers.go`, `util.go`. No behavioral changes.
- **Unified tree renderer**: Replaced `treeNode.modTime` / `treeLine.modTime` field overloading with explicit `annotation` + `showTokens` fields. Deleted `formatSinceTreeOutput` — unified into single `formatTreeOutput`.
- **`displayWidth`** now uses `utf8.RuneCountInString` (idiomatic Go).
- **Git stderr suppressed**: All `exec.Command("git", ...)` calls now set `cmd.Stderr = nil` to prevent git warnings leaking to the user's terminal.
- **Scanning message** now prints to stderr (Unix convention: diagnostics to stderr).
- **Oversize warning** extracted into `checkOutputSize()` helper (was duplicated inline).
- Default `--map` (no value) now excludes config/data files (.yaml, .json, .toml, .env, .gitignore) whose fallback signatures (first 3 lines) are noise.

## [0.3.0] - 2026-04-10

### Added

- **`--since <ref>` flag**: Diff-aware serialization that produces a composite output (map + diff + full content) for files changed between a git ref and HEAD. Accepts branch names, tags, commit hashes, or relative refs like `HEAD~3`. Auto-names output as `dewdrops_since_<ref>.md`.
- **`-o <path>` flag**: Custom output file path for all modes.
- **Oversize output warning**: Warns on stderr when output exceeds ~100K estimated tokens, suggesting `--map` + `--from`.
- **Markdown heading signatures**: `.md` files now extract `#`-headings as signatures instead of falling back to first 3 lines.
- Tests for all new features (15 `--since` tests, 2 `-o` tests, 2 oversize warning tests, 2 batch mod-time tests, 1 markdown signature test).

### Changed

- **Pre-compiled signature regexes**: All regex patterns moved to package-level vars, compiled once at init instead of per-file.
- **Batched git mod-time lookups**: `--map` now runs git mod-time lookups concurrently (16 goroutines) instead of sequentially.
- **Fenced tree block**: `--map` tree output is now wrapped in a ` ```text ` fenced code block for reliable copy-paste into LLM chats.
- `--since` is mutually exclusive with `--map` and `--from`.

## [0.2.0] - 2026-04-10

### Added

- **`--map` flag**: Produces a lightweight structural overview of the repository instead of a full file dump. Includes:
  - Hierarchical tree view with per-file and per-directory token estimates
  - Git last-modified annotations (`[mod: 2 days ago]`)
  - Regex-based signature extraction for Go, Python, JS/TS, Rust, Java/Kotlin, Ruby, PHP, C/C++, SQL, and Shell, with a fallback (first 3 lines) for unknown file types
  - Dedicated map summary (files scanned, signatures extracted, estimated tokens)
- **`--from <paths>` flag**: Serializes only the specified files or directories instead of the entire repo. Accepts comma-separated relative paths. Supports both individual files and directories.
- **`--map --from` combination**: Produces a scoped structural map for a subset of the repository.
- Updated `-h` / `--help` output with new flags, descriptions, and usage examples.
- 15 new test cases covering `--map`, `--from`, their combination, and regression of default behavior.

### Changed

- `Run()` now accepts a `RunOptions` struct instead of a plain output file string, allowing future extensibility.
- Gitignore loading extracted into `loadGitignore()` helper.
- File filtering extracted into `applyFromFilter()` helper.

## [0.1.0] - 2026-02-07

### Added

- Initial release.
- Full repository serialization into a single Markdown file (`dewdrops_context.md`).
- Tree view of repository structure.
- Syntax-highlighted fenced code blocks per file.
- `.gitignore` support via `go-git` library.
- Binary file detection and skipping.
- CLI dump summary (files processed, depth, size, file types).
