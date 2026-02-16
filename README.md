# 💧 DewDrops

[![Release](https://github.com/MedUnes/dewdrops/actions/workflows/release.yml/badge.svg)](https://github.com/MedUnes/dewdrops/actions/workflows/release.yml)
[![Tests](https://github.com/MedUnes/dewdrops/actions/workflows/tests.yaml/badge.svg)](https://github.com/MedUnes/dewdrops/actions/workflows/tests.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/medunes/dewdrops)](https://goreportcard.com/report/github.com/medunes/dewdrops)
[![License](https://img.shields.io/github/license/medunes/dewdrops)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/medunes/dewdrops.svg)](https://pkg.go.dev/github.com/medunes/dewdrops)

**dewdrops** is a high-performance Golang utility designed to "serialize" an entire Git repository into a single Markdown file.

It is built specifically for **Context Stuffing**: delivering a whole codebase to Large Language Models (LLMs) like Claude, Gemini, or GPT, ensuring they understand the file structure and content boundaries without needing direct access to the repository URL.

## ⚡ Features

* **Recursion with Intelligence**: Loops over the entire repo tree.
* **Git-Native Ignore Logic**: Strictly respects `.gitignore` (including nested and global ignores) by leveraging the underlying git index.
* **Insight Report**: Provides a CLI summary of file types, depth, and dump size after execution.
* **Smart Formatting**:
    * **Tree View**: Prints a `tree` command style structure at the very top so the LLM understands the architecture immediately.
    * **Custom Headers**: Uses explicit `### file: path/to/file` headers for clear separation.
    * **Syntax Highlighting**: Wraps content in Markdown code blocks (```go, ```py, etc.) based on file extension.
* **Binary Safety**: Automatically detects and skips binary files (images, compiled binaries) to keep the text dump clean.
* **Air-Gap Ready**: Works entirely locally. No API keys, no cloud uploads, no "access granting."

## 📦 Installation

Since this is a single-file Go program, you can run it directly or build a binary.

### Option 1: Download the lastest release binary (Recommended)
**Run the following command to get the latest release binary**:

```bash
     wget https://raw.githubusercontent.com/MedUnes/dewdrops/master/latest.sh && \
     chmod +x latest.sh && \
     ./latest.sh && \
      rm ./latest.sh
```

### Option 2: Build from Source

You need [Go](https://go.dev/dl/) installed.

1. Clone this repository (or copy the `dewdrops.go` file).
2. Build the binary:
   ```bash
   go build -o dewdrops dewdrops.go
   ```
3. (Optional) Move it to your path:

```bash
mv dewdrops /usr/local/bin/
```

Or simply in one shorthand command:
```bash
make install
```

## 🛠 Usage
You must specify the root directory of the repository you want to dump.

```bash
# Dump current directory
dewdrops .

# Dump a specific repository path
dewdrops /path/to/my-project

# Show help
dewdrops -h
```

### CLI Report Example
After running, dewdrops provides a quick summary of what was packed:

```txt
------------------------------------------------
📦 Dump Summary
------------------------------------------------
Files Processed     : 43
Max Directory Depth : 6
Dump Size           : 0.23 MB (dewdrops_context.md)
File Types:
  .conf       : 1
  .gitignore  : 1
  .go         : 24
  .html       : 1
  .md         : 2
  .mod        : 1
  .no_ext     : 1
  .sum        : 1
  .yaml       : 4
  .yml        : 7
------------------------------------------------
```

### Output File Format
The tool generates a file named dewdrops_context.md.

```markdown
# Repository Context

## Structure
```text
├── cmd/
│   └── main.go
├── pkg/
│   ├── config.go
│   └── utils.go
└── README.md

## File Contents
### file: cmd/main.go
```go
package main
import "fmt"
func main() {
    fmt.Println("Hello LLM")
}
..
..
```

## ❓ Why use this over existing tools?

While tools like `repomix` or `git-ingest` exist, `dewdrops` is designed for **specific LLM prompt engineering** requirements:
1.  **Strict Header Format**: Uses `### file: <path>` inside Markdown, which has shown high efficacy in "Codebase Awareness" prompting.
2.  **Tree-First Approach**: Forces the directory structure to be the first tokens the LLM reads, priming its context window for the file relationships that follow.
3.  **Zero Dependencies**: Compiled into a single static binary. No Node.js `node_modules`, no Python `pip` requirements. Perfect for dropping onto a server or container to extract code.

## 📝 License

MIT