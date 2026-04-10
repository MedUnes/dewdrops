package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type dumpStats struct {
	totalFiles int
	maxDepth   int
	fileTypes  map[string]int
}

type mapStats struct {
	filesScanned int
	sigCount     int
	totalTokens  int
}

type sinceStats struct {
	filesChanged  int
	filesAdded    int
	filesModified int
	filesDeleted  int
	totalTokens   int
	sigCount      int
}

func writeDumpOutput(writer *bufio.Writer, rootDir string, filePaths []string) dumpStats {
	writer.WriteString("# Repository Context\n\n## Structure\n\n```text\n")
	for _, p := range filePaths {
		writer.WriteString(fmt.Sprintf("├── %s\n", p))
	}
	writer.WriteString("```\n\n## File Contents\n\n")

	stats := dumpStats{fileTypes: make(map[string]int)}

	for _, relPath := range filePaths {
		fullPath := filepath.Join(rootDir, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		if !isText(content) {
			continue
		}

		stats.totalFiles++
		depth := strings.Count(relPath, string(os.PathSeparator)) + 1
		if depth > stats.maxDepth {
			stats.maxDepth = depth
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(relPath), "."))
		if ext == "" {
			ext = "no_ext"
		}
		stats.fileTypes[ext]++

		writer.WriteString(fmt.Sprintf("### file: %s\n", relPath))
		writer.WriteString(fmt.Sprintf("```%s\n", getLanguageID(ext)))
		writer.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			writer.WriteString("\n")
		}
		writer.WriteString("```\n\n")
	}

	return stats
}

func writeMapOutput(writer *bufio.Writer, rootDir string, filePaths []string, mapFilter string) mapStats {
	var stats mapStats

	filePaths = filterMapExts(filePaths, mapFilter)

	tokenMap := make(map[string]int)
	sigMap := make(map[string][]string)
	modTimeMap := batchGitModTimes(rootDir, filePaths)

	for _, relPath := range filePaths {
		fullPath := filepath.Join(rootDir, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		stats.filesScanned++
		tokens := estimateTokens(content)
		tokenMap[relPath] = tokens
		stats.totalTokens += tokens

		if isText(content) {
			sigs := extractSignatures(relPath, content)
			if len(sigs) > 0 {
				sigMap[relPath] = sigs
				stats.sigCount += len(sigs)
			}
		}
	}

	dirName := filepath.Base(rootDir)
	if dirName == "." {
		if wd, err := os.Getwd(); err == nil {
			dirName = filepath.Base(wd)
		}
	}
	writer.WriteString(fmt.Sprintf("# Repository Map: %s\n", dirName))
	writer.WriteString(fmt.Sprintf("%d files | ~%s tokens (estimated)\n\n", stats.filesScanned, formatNumber(stats.totalTokens)))

	writer.WriteString("## Structure\n\n```text\n")
	root := buildTree(filePaths, tokenMap, modTimeMap)
	lines := renderTreeLines(root)
	writer.WriteString(formatTreeOutput(lines))
	writer.WriteString("```\n\n")

	writer.WriteString("## Signatures\n\n")
	for _, relPath := range filePaths {
		sigs, ok := sigMap[relPath]
		if !ok || len(sigs) == 0 {
			continue
		}
		writer.WriteString(fmt.Sprintf("### %s\n", relPath))
		for _, sig := range sigs {
			writer.WriteString(sig + "\n")
		}
		writer.WriteString("\n")
	}

	return stats
}

func writeSinceOutput(writer *bufio.Writer, rootDir string, ref string) (sinceStats, error) {
	var stats sinceStats

	cmd := exec.Command("git", "rev-parse", "--short", ref)
	cmd.Dir = rootDir
	cmd.Stderr = nil
	if _, err := cmd.Output(); err != nil {
		return stats, fmt.Errorf("Invalid git ref: %s", ref)
	}

	changes := gitDiffNameStatus(rootDir, ref)
	if len(changes) == 0 {
		fmt.Fprintf(os.Stderr, "⚠️  No changes found between %s and HEAD.\n", ref)
		return stats, nil
	}

	stats.filesChanged = len(changes)

	for _, c := range changes {
		switch c.Status {
		case "A":
			stats.filesAdded++
		case "M":
			stats.filesModified++
		case "D":
			stats.filesDeleted++
		}
	}

	tokenMap := make(map[string]int)
	sigMap := make(map[string][]string)
	contentMap := make(map[string][]byte)
	statusMap := make(map[string]string)
	var nonDeletedPaths []string

	for _, c := range changes {
		statusMap[c.Path] = c.Status
		if c.OldPath != "" {
			statusMap[c.Path] = "R"
		}
		if c.Status == "D" {
			continue
		}
		fullPath := filepath.Join(rootDir, c.Path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		if !isText(content) {
			continue
		}
		nonDeletedPaths = append(nonDeletedPaths, c.Path)
		contentMap[c.Path] = content
		tokens := estimateTokens(content)
		tokenMap[c.Path] = tokens
		stats.totalTokens += tokens

		sigs := extractSignatures(c.Path, content)
		if len(sigs) > 0 {
			sigMap[c.Path] = sigs
			stats.sigCount += len(sigs)
		}
	}

	sort.Strings(nonDeletedPaths)

	shortHead := gitShortHash(rootDir, "HEAD")
	shortRef := gitShortHash(rootDir, ref)
	writer.WriteString(fmt.Sprintf("# Changes: %s vs %s\n", shortHead, shortRef))
	writer.WriteString(fmt.Sprintf("%d files changed | ~%s tokens (estimated)\n\n", stats.filesChanged, formatNumber(stats.totalTokens)))

	writer.WriteString("## Map of Changed Files\n\n```text\n")
	sinceTree := buildSinceTree(changes, tokenMap, statusMap)
	sinceLines := renderTreeLines(sinceTree)
	writer.WriteString(formatTreeOutput(sinceLines))
	writer.WriteString("```\n\n")

	for _, path := range nonDeletedPaths {
		sigs, ok := sigMap[path]
		if !ok || len(sigs) == 0 {
			continue
		}
		writer.WriteString(fmt.Sprintf("### %s\n", path))
		for _, sig := range sigs {
			writer.WriteString(sig + "\n")
		}
		writer.WriteString("\n")
	}

	writer.WriteString("## Diff\n\n```diff\n")
	writer.WriteString(gitDiff(rootDir, ref))
	writer.WriteString("```\n\n")

	writer.WriteString("## File Contents\n\n")
	for _, c := range changes {
		if c.Status == "D" {
			continue
		}
		content, ok := contentMap[c.Path]
		if !ok {
			continue
		}
		statusLabel := ""
		switch {
		case c.OldPath != "":
			statusLabel = fmt.Sprintf(" [RENAMED from %s]", c.OldPath)
		case c.Status == "M":
			statusLabel = " [MODIFIED]"
		case c.Status == "A":
			statusLabel = " [ADDED]"
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(c.Path), "."))
		if ext == "" {
			ext = "no_ext"
		}
		writer.WriteString(fmt.Sprintf("### file: %s%s\n", c.Path, statusLabel))
		writer.WriteString(fmt.Sprintf("```%s\n", getLanguageID(ext)))
		writer.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			writer.WriteString("\n")
		}
		writer.WriteString("```\n\n")
	}

	return stats, nil
}
