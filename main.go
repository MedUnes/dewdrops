package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const DefaultOutputFileName = "dewdrops_context.md"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <repository_root>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Error: Missing required argument <repository_root>")
		flag.Usage()
		os.Exit(1)
	}

	rootDir := flag.Arg(0)
	if err := Run(rootDir, DefaultOutputFileName); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func Run(rootDir string, outputFile string) error {
	info, err := os.Stat(rootDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("'%s' is not a valid directory", rootDir)
	}

	fmt.Printf("💧 dewdrops: Scanning '%s'...\n", rootDir)

	var matcher gitignore.Matcher
	ignorePath := filepath.Join(rootDir, ".gitignore")
	if _, err := os.Stat(ignorePath); err == nil {
		f, err := os.Open(ignorePath)
		if err == nil {
			defer f.Close()
			patterns := make([]gitignore.Pattern, 0)
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
					continue
				}
				pattern := gitignore.ParsePattern(line, nil)
				if pattern != nil {
					patterns = append(patterns, pattern)
				}
			}
			matcher = gitignore.NewMatcher(patterns)
		}
	}

	var filePaths []string

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(rootDir, path)
		if relPath == "." {
			return nil
		}

		pathParts := strings.Split(relPath, string(os.PathSeparator))
		for _, p := range pathParts {
			if p == ".git" {
				return filepath.SkipDir
			}
		}

		if matcher != nil {
			isDir := info.IsDir()
			if matcher.Match(pathParts, isDir) {
				if isDir {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			return nil
		}
		if info.Name() == outputFile {
			return nil
		}

		filePaths = append(filePaths, relPath)
		return nil
	})

	if err != nil {
		return err
	}
	if len(filePaths) == 0 {
		return fmt.Errorf("no files found in %s", rootDir)
	}

	sort.Strings(filePaths)

	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()
	writer := bufio.NewWriter(outFile)

	writer.WriteString("# Repository Context\n\n## Structure\n\n```text\n")

	for _, p := range filePaths {
		writer.WriteString(fmt.Sprintf("├── %s\n", p))
	}
	writer.WriteString("```\n\n## File Contents\n\n")

	var totalFiles, maxDepth int
	fileTypes := make(map[string]int)

	for _, relPath := range filePaths {
		fullPath := filepath.Join(rootDir, relPath)
		content, err := ioutil.ReadFile(fullPath)
		if err != nil {
			continue
		}
		if !isText(content) {
			continue
		}

		totalFiles++

		depth := strings.Count(relPath, string(os.PathSeparator)) + 1
		if depth > maxDepth {
			maxDepth = depth
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(relPath), "."))
		if ext == "" {
			ext = "no_ext"
		}
		fileTypes[ext]++

		writer.WriteString(fmt.Sprintf("### file: %s\n", relPath))
		writer.WriteString(fmt.Sprintf("```%s\n", getLanguageID(ext)))
		writer.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			writer.WriteString("\n")
		}
		writer.WriteString("```\n\n")
	}
	writer.Flush()

	fi, _ := outFile.Stat()
	fileSizeMB := float64(fi.Size()) / 1024 / 1024

	fmt.Println("\n------------------------------------------------")
	fmt.Printf("📦 Dump Summary\n")
	fmt.Println("------------------------------------------------")
	fmt.Printf("Files Processed     : %d\n", totalFiles)
	fmt.Printf("Max Directory Depth : %d\n", maxDepth)
	fmt.Printf("Dump Size           : %.2f MB (%s)\n", fileSizeMB, outputFile)
	fmt.Println("")

	fmt.Println("File Types:")

	keys := make([]string, 0, len(fileTypes))
	for k := range fileTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		// Formatted exactly as requested: .ext : count
		fmt.Printf("  .%-10s : %d\n", k, fileTypes[k])
	}
	fmt.Println("------------------------------------------------")

	return nil
}

func getLanguageID(ext string) string {
	switch ext {
	case "go":
		return "go"
	case "rs":
		return "rust"
	case "js", "ts":
		return "javascript"
	case "py":
		return "python"
	case "md":
		return "markdown"
	case "html":
		return "html"
	case "css":
		return "css"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "sql":
		return "sql"
	case "sh":
		return "bash"
	case "dockerfile":
		return "dockerfile"
	case "tf":
		return "hcl"
	default:
		return "txt"
	}
}

func isText(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}
