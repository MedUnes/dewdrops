# DewDrops

[![Release](https://github.com/MedUnes/dewdrops/actions/workflows/release.yml/badge.svg)](https://github.com/MedUnes/dewdrops/actions/workflows/release.yml)
[![Tests](https://github.com/MedUnes/dewdrops/actions/workflows/tests.yaml/badge.svg)](https://github.com/MedUnes/dewdrops/actions/workflows/tests.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/medunes/dewdrops)](https://goreportcard.com/report/github.com/medunes/dewdrops)
[![License](https://img.shields.io/github/license/medunes/dewdrops)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/medunes/dewdrops.svg)](https://pkg.go.dev/github.com/medunes/dewdrops)

**dewdrops** is a high-performance Golang utility designed to "serialize" an entire Git repository into a single
Markdown file.

It is built specifically for **Context Stuffing**: delivering a whole codebase to Large Language Models (LLMs) like
Claude, Gemini, or GPT, ensuring they understand the file structure and content boundaries without needing direct access
to the repository URL.

## Features

* **Full Repo Dump**: Serializes every file into a single Markdown file with tree view, headers, and syntax-highlighted
  code blocks.
* **Repo Map (`--map`)**: Produces a lightweight structural overview -- tree with token estimates and extracted
  function/type/class signatures -- instead of full file contents. Optionally filter by extension.
* **Scoped Selection (`--from`)**: Dumps only the files or directories you specify, instead of the entire repo.
* **Change Review (`--since`)**: Generates a review-ready document for recent changes -- map of changed files, full
  diff, and complete content of every changed file in one paste.
* **Custom Output (`-o`)**: Write output to any path instead of the default `dewdrops_context.md`.
* **Git-Native Ignore Logic**: Strictly respects `.gitignore` by leveraging the underlying git index.
* **Binary Safety**: Automatically detects and skips binary files.
* **Smart Formatting**:
    * **Tree View**: Prints a directory structure at the top so the LLM understands the architecture immediately.
    * **Custom Headers**: Uses explicit `### file: path/to/file` headers for clear separation.
    * **Syntax Highlighting**: Wraps content in Markdown code blocks (` ```go `, ` ```py `, etc.) based on file
      extension.
* **Oversize Warning**: Warns when output exceeds ~100K tokens, suggesting `--map` + `--from` instead.
* **Air-Gap Ready**: Works entirely locally. No API keys, no cloud uploads, no "access granting."

## Installation

### Option 1: Download the latest release binary (Recommended)

```bash
wget https://raw.githubusercontent.com/MedUnes/dewdrops/master/latest.sh && \
chmod +x latest.sh && \
./latest.sh && \
rm ./latest.sh
```

### Option 2: Build from Source

You need [Go 1.25+](https://go.dev/dl/) installed.

```bash
git clone https://github.com/MedUnes/dewdrops.git
cd dewdrops
make build
sudo mv dewdrops /usr/local/bin/
```

Or in one step:

```bash
make install
```

## Usage

```
Usage: dewdrops [options] <repo-path>

Options:
  --map[=exts]       Output structural map (tree + signatures + token estimates).
                     Optionally filter by extensions (comma-separated, e.g.
                     --map=go,py). Default: supported languages only.
                     Use --map=any for all text files.
  --from <paths>     Only include specified files/dirs (comma-separated, relative
                     to repo root). Example: --from src/main.go,internal/auth/
  --since <ref>      Diff-aware output: map + diff + content for files changed
                     between <ref> and HEAD. Accepts branch names, tags, commit
                     hashes, or relative refs like HEAD~3.
                     Cannot be combined with --map or --from.
  -o <path>          Output file path (default: dewdrops_context.md)
  --version          Print version and exit
  -h, --help         Show this help message

Examples:
  dewdrops .                                        # Full repo dump
  dewdrops --map .                                  # Structural overview only
  dewdrops --map=go,py .                            # Map only Go and Python files
  dewdrops --map=any .                              # Map all text files
  dewdrops --from internal/auth/ .                  # Dump specific directory
  dewdrops --map --from internal/auth/,cmd/ .       # Map of specific subtree
  dewdrops --since main .                           # Review changes vs main
  dewdrops --since HEAD~3 -o review.md .            # Last 3 commits, custom path
```

## Flag reference

### Full repo dump (default)

Serialize the entire repository into `dewdrops_context.md`:

```bash
dewdrops .
dewdrops /path/to/my-project
```

This produces the full dump: tree view at the top, followed by every file's content in fenced code blocks with syntax
highlighting.

**CLI summary:**

```
------------------------------------------------
Dump Summary
------------------------------------------------
Files Processed     : 43
Max Directory Depth : 6
Dump Size           : 0.23 MB (dewdrops_context.md)

File Types:
  .go         : 24
  .md         : 2
  .yaml       : 4
------------------------------------------------
```

### Repo map (`--map`)

Produce a lightweight structural overview instead of dumping full file contents. This gives an LLM enough context to
understand the architecture and ask for specific files.

`--map` accepts an optional extension filter:

| Usage                    | Behavior                                                                                                                                                                             |
|--------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dewdrops --map .`       | Map files with supported signature patterns only (Go, Python, JS/TS, Rust, Java/Kotlin, Ruby, PHP, C/C++, Shell, SQL, Markdown). Config files like .yaml, .json, .toml are excluded. |
| `dewdrops --map=go,py .` | Map only files with .go and .py extensions.                                                                                                                                          |
| `dewdrops --map=any .`   | Map all text files, including config/data files. Unknown extensions fall back to first 3 lines as signatures.                                                                        |

The map includes:

- **Token estimates** per file and per directory (aggregated), using the `len(bytes) / 4` heuristic
- **Last-modified time** from git history (e.g. `[mod: 2 days ago]`)
- **Extracted signatures**: function, type, class, and interface declarations, extracted via regex per language.
  Supports Go, Python, JS/TS, Rust, Java/Kotlin, Ruby, PHP, C/C++, SQL, Shell, and Markdown headings. For PHP,
  Java/Kotlin, Ruby, JS/TS, and Rust, indented class/impl methods are captured. Unknown file types fall back to the
  first 3 non-empty lines.

**CLI summary:**

```
------------------------------------------------
Map Summary
------------------------------------------------
Files Scanned       : 43
Signatures Extracted: 127
Estimated Tokens    : 24,800
Output Size         : 0.012 MB (dewdrops_context.md)
------------------------------------------------
```

### Scoped selection (`--from`)

Dump only specific files or directories instead of the entire repo:

```bash
dewdrops --from main.go .                          # Single file
dewdrops --from internal/auth/jwt.go,internal/auth/middleware.go .  # Multiple files
dewdrops --from internal/auth/ .                   # Entire directory
```

The output format is identical to a full dump, but scoped to only the requested paths. The tree view also shows only the
selected files.

If a path doesn't exist or is gitignored, a warning is printed to stderr and the remaining paths are still processed. If
none of the specified paths are valid, dewdrops exits with code 1.

### Change review (`--since`)

Generate a review-ready document for recent changes:

```bash
dewdrops --since main .          # Changes vs main branch
dewdrops --since HEAD~3 .        # Last 3 commits
dewdrops --since a1b2c3d .       # Since a specific commit
```

Output contains three sections:

1. **Map** -- tree with change status (`[M]`odified, `[A]`dded, `[D]`eleted) and signatures of changed files
2. **Diff** -- full unified diff in a fenced code block
3. **Content** -- complete current content of all non-deleted changed files, with status labels (`[MODIFIED]`,
   `[ADDED]`, `[RENAMED from ...]`)

Output is auto-named `dewdrops_since_<ref>.md` unless `-o` is specified. For example, `dewdrops --since main .` writes
to `dewdrops_since_main.md`.

`--since` cannot be combined with `--map` or `--from` -- it produces its own composite output.

**CLI summary:**

```
------------------------------------------------
Since Summary (a1b2c3d vs f4e5d6c)
------------------------------------------------
Files Changed       : 4 (2 modified, 1 added, 1 deleted)
Signatures Extracted: 12
Estimated Tokens    : 8,400
Output Size         : 0.034 MB (dewdrops_since_main.md)
------------------------------------------------
```

### Custom output path (`-o`)

Write output to a specific file instead of the default:

```bash
dewdrops -o /tmp/context.md .
dewdrops --map -o map.md .
dewdrops --since main -o review.md .
```

Works with all modes. If omitted, defaults to `dewdrops_context.md` (or `dewdrops_since_<ref>.md` for `--since`).

### Version

```bash
dewdrops --version
```

Prints the version string. Release binaries show the version tag (e.g. `dewdrops v0.3.1`). Local builds show
`dewdrops dev`.

## Flag combination matrix

| Flags                                    | Behavior                                   |
|------------------------------------------|--------------------------------------------|
| `dewdrops .`                             | Full dump of all files                     |
| `dewdrops --map .`                       | Map with supported extensions only         |
| `dewdrops --map=any .`                   | Map with all text files                    |
| `dewdrops --map=go,py .`                 | Map with only .go and .py files            |
| `dewdrops --from a.go,b.go .`            | Full dump scoped to specified files        |
| `dewdrops --from internal/ .`            | Full dump scoped to a directory            |
| `dewdrops --map --from a.go .`           | Map scoped to specified files              |
| `dewdrops --map=go --from internal/ .`   | Map of .go files in a directory            |
| `dewdrops --since main .`                | Composite: map + diff + content of changes |
| `dewdrops --since HEAD~3 -o review.md .` | Composite, custom output path              |
| `dewdrops -o out.md .`                   | Full dump, custom output path              |
| `dewdrops -o out.md --map .`             | Map, custom output path                    |
| `dewdrops --since X --map .`             | **ERROR**: mutually exclusive              |
| `dewdrops --since X --from Y .`          | **ERROR**: mutually exclusive              |
| `dewdrops --version`                     | Print version and exit                     |

## Typical workflow

1. **Get the big picture**: Run `dewdrops --map .` and paste the output into your LLM chat.
2. **LLM asks for details**: The LLM sees the full structure and signatures, then asks for specific files (e.g. "I need
   to see `internal/auth/middleware.go` and `internal/store/postgres.go`").
3. **Provide the files**: Run `dewdrops --from internal/auth/middleware.go,internal/store/postgres.go .` and paste the
   result.

This two-step approach can reduce token usage by 90%+ compared to dumping the entire repo upfront.

For code reviews, use `dewdrops --since main .` to generate a self-contained review document and paste it into your LLM
with "Review these changes."

## Why use this over existing tools?

While tools like `repomix` or `git-ingest` exist, `dewdrops` is designed for **specific LLM prompt engineering**
requirements:

1. **Strict Header Format**: Uses `### file: <path>` inside Markdown, which has shown high efficacy in "Codebase
   Awareness" prompting.
2. **Tree-First Approach**: Forces the directory structure to be the first tokens the LLM reads, priming its context
   window for the file relationships that follow.
3. **Map Mode**: Unique lightweight overview mode with token estimates and signature extraction, enabling a two-step
   workflow that drastically cuts token usage.
4. **Diff-Aware Review**: `--since` produces a self-contained review document (map + diff + content) optimized for LLM
   code review.
5. **Zero Dependencies**: Compiled into a single static binary. No Node.js `node_modules`, no Python `pip` requirements.
   Perfect for dropping onto a server or container to extract code.

## License

MIT
