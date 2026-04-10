# Changelog

All notable changes to DewDrops will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

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

## [0.1.0] - 2025-02-07

### Added

- Initial release.
- Full repository serialization into a single Markdown file (`dewdrops_context.md`).
- Tree view of repository structure.
- Syntax-highlighted fenced code blocks per file.
- `.gitignore` support via `go-git` library.
- Binary file detection and skipping.
- CLI dump summary (files processed, depth, size, file types).
