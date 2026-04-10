package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const warnTokenThreshold = 100_000

var supportedMapExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".rs": true, ".java": true, ".kt": true, ".rb": true, ".php": true,
	".c": true, ".h": true, ".cpp": true, ".hpp": true,
	".sh": true, ".bash": true,
	".sql": true, ".md": true, ".markdown": true,
}

func filterMapExts(filePaths []string, filter string) []string {
	if filter == "any" {
		return filePaths
	}

	var allowed map[string]bool
	if filter == "" {
		allowed = supportedMapExts
	} else {
		allowed = make(map[string]bool)
		for _, ext := range strings.Split(filter, ",") {
			ext = strings.TrimSpace(ext)
			if ext != "" {
				if !strings.HasPrefix(ext, ".") {
					ext = "." + ext
				}
				allowed[strings.ToLower(ext)] = true
			}
		}
	}

	var filtered []string
	for _, fp := range filePaths {
		ext := strings.ToLower(filepath.Ext(fp))
		if allowed[ext] {
			filtered = append(filtered, fp)
		}
	}
	return filtered
}

func loadGitignore(rootDir string) gitignore.Matcher {
	ignorePath := filepath.Join(rootDir, ".gitignore")
	f, err := os.Open(ignorePath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if p := gitignore.ParsePattern(line, nil); p != nil {
			patterns = append(patterns, p)
		}
	}
	if len(patterns) == 0 {
		return nil
	}
	return gitignore.NewMatcher(patterns)
}

func isText(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}

func estimateTokens(content []byte) int {
	return len(content) / 4
}

func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
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

func displayWidth(s string) int {
	return utf8.RuneCountInString(s)
}
