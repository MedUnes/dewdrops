# DewDrops

[![Release](https://github.com/MedUnes/dewdrops/actions/workflows/release.yml/badge.svg)](https://github.com/MedUnes/dewdrops/actions/workflows/release.yml)
[![Tests](https://github.com/MedUnes/dewdrops/actions/workflows/tests.yaml/badge.svg)](https://github.com/MedUnes/dewdrops/actions/workflows/tests.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/medunes/dewdrops)](https://goreportcard.com/report/github.com/medunes/dewdrops)
[![License](https://img.shields.io/github/license/medunes/dewdrops)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/medunes/dewdrops.svg)](https://pkg.go.dev/github.com/medunes/dewdrops)

**dewdrops** is a high-performance Golang utility designed to "serialize" an entire Git repository into a single Markdown file.

It is built specifically for **Context Stuffing**: delivering a whole codebase to Large Language Models (LLMs) like Claude, Gemini, or GPT, ensuring they understand the file structure and content boundaries without needing direct access to the repository URL.

## Features

* **Full Repo Dump**: Serializes every file into a single Markdown file with tree view, headers, and syntax-highlighted code blocks.
* **Repo Map (`--map`)**: Produces a lightweight structural overview -- tree with token estimates and extracted function/type/class signatures -- instead of full file contents. Gives an LLM architectural awareness in ~2-5% of the tokens.
* **Scoped Selection (`--from`)**: Dumps only the files or directories you specify, instead of the entire repo. Perfect for follow-up prompts after reviewing a map.
* **Git-Native Ignore Logic**: Strictly respects `.gitignore` by leveraging the underlying git index.
* **Binary Safety**: Automatically detects and skips binary files.
* **Smart Formatting**:
    * **Tree View**: Prints a directory structure at the top so the LLM understands the architecture immediately.
    * **Custom Headers**: Uses explicit `### file: path/to/file` headers for clear separation.
    * **Syntax Highlighting**: Wraps content in Markdown code blocks (` ```go `, ` ```py `, etc.) based on file extension.
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
  --map              Output structural map (tree + signatures + token estimates)
                     instead of full file contents
  --from <paths>     Only include specified files/dirs (comma-separated, relative
                     to repo root). Example: --from src/main.go,internal/auth/
  -h, --help         Show this help message
```

### Full repo dump (default)

Serialize the entire repository into `dewdrops_context.md`:

```bash
dewdrops .
dewdrops /path/to/my-project
```

This produces the full dump: tree view at the top, followed by every file's content in fenced code blocks with syntax highlighting.

**Output example:**

```markdown
# Repository Context

## Structure

```text
├── cmd/server/main.go
├── internal/auth/jwt.go
├── internal/auth/middleware.go
├── internal/store/postgres.go
└── go.mod
```

## File Contents

### file: cmd/server/main.go
```go
package main

func main() {
    // ...
}
```

### file: internal/auth/jwt.go
```go
package auth
// ...
```
```

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

Produce a lightweight structural overview instead of dumping full file contents. This gives an LLM enough context to understand the architecture and ask for specific files:

```bash
dewdrops --map .
```

**Output example:**

```markdown
# Repository Map: my-project
5 files | ~10,500 tokens (estimated)

## Structure

├── cmd/server/                    (~1,200 tok)
│   └── main.go                    (~1,200 tok)  [mod: 1 day ago]
├── internal/auth/                 (~4,200 tok)
│   ├── jwt.go                     (~2,400 tok)  [mod: 2 days ago]
│   └── middleware.go              (~1,800 tok)
├── internal/store/                (~5,100 tok)
│   └── postgres.go                (~5,100 tok)
└── go.mod                         (~50 tok)

## Signatures

### cmd/server/main.go
func main()
func setupRouter() *http.ServeMux

### internal/auth/jwt.go
type Claims struct {
func NewToken(user User) (string, error)
func ValidateToken(raw string) (*Claims, error)

### internal/auth/middleware.go
func RequireAuth(next http.Handler) http.Handler
```

The map includes:
- **Token estimates** per file and per directory (aggregated), using the `len(bytes) / 4` heuristic
- **Last-modified time** from git history (e.g. `[mod: 2 days ago]`)
- **Extracted signatures**: function, type, class, and interface declarations, extracted via regex per language (Go, Python, JS/TS, Rust, Java/Kotlin, Ruby, PHP, C/C++, SQL, Shell). Unknown file types fall back to the first 3 non-empty lines.

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
# Single file
dewdrops --from main.go .

# Multiple files
dewdrops --from internal/auth/jwt.go,internal/auth/middleware.go .

# Entire directory
dewdrops --from internal/auth/ .
```

The output format is identical to a full dump, but scoped to only the requested paths. The tree view also shows only the selected files.

If a path doesn't exist or is gitignored, a warning is printed to stderr and the remaining paths are still processed:

```
Skipped (not found or ignored): nonexistent.go
```

If none of the specified paths are valid, dewdrops exits with code 1.

### Combining `--map` and `--from`

Get a structural overview of just the files you care about:

```bash
dewdrops --map --from internal/auth/,cmd/ .
```

This produces a map (tree + signatures + tokens) scoped to only the specified paths. Useful for getting a quick overview of a subsystem before committing to a full dump.

## Typical workflow

1. **Get the big picture**: Run `dewdrops --map .` and paste the output into your LLM chat.
2. **LLM asks for details**: The LLM sees the full structure and signatures, then asks for specific files (e.g. "I need to see `internal/auth/middleware.go` and `internal/store/postgres.go`").
3. **Provide the files**: Run `dewdrops --from internal/auth/middleware.go,internal/store/postgres.go .` and paste the result.

This two-step approach can reduce token usage by 90%+ compared to dumping the entire repo upfront.

## Why use this over existing tools?

While tools like `repomix` or `git-ingest` exist, `dewdrops` is designed for **specific LLM prompt engineering** requirements:
1. **Strict Header Format**: Uses `### file: <path>` inside Markdown, which has shown high efficacy in "Codebase Awareness" prompting.
2. **Tree-First Approach**: Forces the directory structure to be the first tokens the LLM reads, priming its context window for the file relationships that follow.
3. **Map Mode**: Unique lightweight overview mode with token estimates and signature extraction, enabling a two-step workflow that drastically cuts token usage.
4. **Zero Dependencies**: Compiled into a single static binary. No Node.js `node_modules`, no Python `pip` requirements. Perfect for dropping onto a server or container to extract code.

## License

MIT
