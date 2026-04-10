package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const DefaultOutputFileName = "dewdrops_context.md"

// RunOptions configures the behavior of Run.
type RunOptions struct {
	MapMode    bool
	FromPaths  []string
	OutputFile string
}

func main() {
	mapFlag := flag.Bool("map", false, "Output structural map instead of full file contents")
	fromFlag := flag.String("from", "", "Comma-separated list of file/dir paths to include")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: dewdrops [options] <repo-path>

Options:
  --map              Output structural map (tree + signatures + token estimates)
                     instead of full file contents
  --from <paths>     Only include specified files/dirs (comma-separated, relative
                     to repo root). Example: --from src/main.go,internal/auth/
  -h, --help         Show this help message

Examples:
  dewdrops .                                        # Full repo dump
  dewdrops --map .                                  # Structural overview only
  dewdrops --from internal/auth/ .                  # Dump specific directory
  dewdrops --map --from internal/auth/,cmd/ .       # Map of specific subtree
`)
	}
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: Missing required argument <repo-path>")
		flag.Usage()
		os.Exit(1)
	}

	rootDir := flag.Arg(0)
	opts := RunOptions{
		MapMode:    *mapFlag,
		OutputFile: DefaultOutputFileName,
	}
	if *fromFlag != "" {
		opts.FromPaths = strings.Split(*fromFlag, ",")
	}

	if err := Run(rootDir, opts); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

func Run(rootDir string, opts RunOptions) error {
	info, err := os.Stat(rootDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("'%s' is not a valid directory", rootDir)
	}

	outputFile := opts.OutputFile
	if outputFile == "" {
		outputFile = DefaultOutputFileName
	}
	outputBase := filepath.Base(outputFile)

	fmt.Printf("dewdrops: Scanning '%s'💧💧💧\n", rootDir)

	matcher := loadGitignore(rootDir)

	var allFilePaths []string
	err = filepath.Walk(rootDir, func(path string, fi os.FileInfo, err error) error {
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
			if matcher.Match(pathParts, fi.IsDir()) {
				if fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if fi.IsDir() {
			return nil
		}
		if fi.Name() == outputBase {
			return nil
		}
		allFilePaths = append(allFilePaths, relPath)
		return nil
	})
	if err != nil {
		return err
	}

	filePaths, err := applyFromFilter(allFilePaths, opts.FromPaths)
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

	if opts.MapMode {
		stats := writeMapOutput(writer, rootDir, filePaths)
		writer.Flush()
		fi, _ := outFile.Stat()
		fileSizeMB := float64(fi.Size()) / 1024 / 1024
		fmt.Println("\n------------------------------------------------")
		fmt.Println("Map Summary")
		fmt.Println("------------------------------------------------")
		fmt.Printf("Files Scanned       : %d\n", stats.filesScanned)
		fmt.Printf("Signatures Extracted: %d\n", stats.sigCount)
		fmt.Printf("Estimated Tokens    : %s\n", formatNumber(stats.totalTokens))
		fmt.Printf("Output Size         : %.3f MB (%s)\n", fileSizeMB, outputFile)
		fmt.Println("------------------------------------------------")
	} else {
		stats := writeDumpOutput(writer, rootDir, filePaths)
		writer.Flush()
		fi, _ := outFile.Stat()
		fileSizeMB := float64(fi.Size()) / 1024 / 1024
		fmt.Println("\n------------------------------------------------")
		fmt.Printf("Dump Summary\n")
		fmt.Println("------------------------------------------------")
		fmt.Printf("Files Processed     : %d\n", stats.totalFiles)
		fmt.Printf("Max Directory Depth : %d\n", stats.maxDepth)
		fmt.Printf("Dump Size           : %.2f MB (%s)\n", fileSizeMB, outputFile)
		fmt.Println("")
		fmt.Println("File Types:")
		keys := make([]string, 0, len(stats.fileTypes))
		for k := range stats.fileTypes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  .%-10s : %d\n", k, stats.fileTypes[k])
		}
		fmt.Println("------------------------------------------------")
	}

	return nil
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

func applyFromFilter(allPaths []string, fromPaths []string) ([]string, error) {
	if len(fromPaths) == 0 {
		return allPaths, nil
	}

	cleaned := make([]string, len(fromPaths))
	for i, p := range fromPaths {
		cleaned[i] = filepath.Clean(strings.TrimSpace(p))
	}

	var filtered []string
	matched := make(map[int]bool)

	for _, fp := range allPaths {
		for i, from := range cleaned {
			if fp == from || strings.HasPrefix(fp, from+"/") {
				filtered = append(filtered, fp)
				matched[i] = true
				break
			}
		}
	}

	for i, from := range cleaned {
		if !matched[i] {
			fmt.Fprintf(os.Stderr, "Skipped (not found or ignored): %s\n", from)
		}
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("No valid files found for the given --from paths.")
	}

	seen := make(map[string]bool)
	var deduped []string
	for _, p := range filtered {
		if !seen[p] {
			seen[p] = true
			deduped = append(deduped, p)
		}
	}
	return deduped, nil
}

type dumpStats struct {
	totalFiles int
	maxDepth   int
	fileTypes  map[string]int
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

type mapStats struct {
	filesScanned int
	sigCount     int
	totalTokens  int
}

type treeNode struct {
	name     string
	isDir    bool
	children []*treeNode
	tokens   int
	modTime  string
}

type treeLine struct {
	prefix  string
	tokens  int
	modTime string
	isDir   bool
}

func writeMapOutput(writer *bufio.Writer, rootDir string, filePaths []string) mapStats {
	var stats mapStats

	tokenMap := make(map[string]int)
	modTimeMap := make(map[string]string)
	sigMap := make(map[string][]string)

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

		modTimeMap[relPath] = getGitModTime(rootDir, relPath)

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

	writer.WriteString("## Structure\n\n")
	root := buildTree(filePaths, tokenMap, modTimeMap)
	lines := renderTreeLines(root)
	writer.WriteString(formatTreeOutput(lines))
	writer.WriteString("\n")

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

func buildTree(filePaths []string, tokenMap map[string]int, modTimeMap map[string]string) *treeNode {
	root := &treeNode{isDir: true}
	for _, path := range filePaths {
		parts := strings.Split(path, "/")
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			var child *treeNode
			for _, c := range current.children {
				if c.name == part {
					child = c
					break
				}
			}
			if child == nil {
				child = &treeNode{
					name:  part,
					isDir: !isLast,
				}
				if isLast {
					child.tokens = tokenMap[path]
					child.modTime = modTimeMap[path]
				}
				current.children = append(current.children, child)
			}
			current = child
		}
	}
	aggregateTokens(root)
	return root
}

func aggregateTokens(node *treeNode) int {
	if !node.isDir {
		return node.tokens
	}
	total := 0
	for _, child := range node.children {
		total += aggregateTokens(child)
	}
	node.tokens = total
	return total
}

func renderTreeLines(root *treeNode) []treeLine {
	var lines []treeLine
	renderChildren(root, "", &lines)
	return lines
}

func renderChildren(node *treeNode, prefix string, lines *[]treeLine) {
	for i, child := range node.children {
		isLast := i == len(node.children)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		name := child.name
		if child.isDir {
			name += "/"
		}

		*lines = append(*lines, treeLine{
			prefix:  prefix + connector + name,
			tokens:  child.tokens,
			modTime: child.modTime,
			isDir:   child.isDir,
		})

		if child.isDir {
			childPrefix := prefix + "│   "
			if isLast {
				childPrefix = prefix + "    "
			}
			renderChildren(child, childPrefix, lines)
		}
	}
}

func formatTreeOutput(lines []treeLine) string {
	maxWidth := 0
	for _, l := range lines {
		w := displayWidth(l.prefix)
		if w > maxWidth {
			maxWidth = w
		}
	}

	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l.prefix)
		padding := maxWidth - displayWidth(l.prefix) + 2
		sb.WriteString(strings.Repeat(" ", padding))
		sb.WriteString(fmt.Sprintf("(~%s tok)", formatNumber(l.tokens)))
		if !l.isDir && l.modTime != "" {
			sb.WriteString(fmt.Sprintf("  [mod: %s]", l.modTime))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func displayWidth(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func extractSignatures(filePath string, content []byte) []string {
	ext := strings.ToLower(filepath.Ext(filePath))
	lines := strings.Split(string(content), "\n")

	patterns, cMode, fallback := sigPatternsForExt(ext)

	if fallback {
		var result []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				result = append(result, trimmed)
				if len(result) >= 3 {
					break
				}
			}
		}
		return result
	}

	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		matched := false
		for _, pat := range patterns {
			if pat.MatchString(line) {
				result = append(result, trimmed)
				matched = true
				break
			}
		}

		if !matched && cMode {
			if strings.Contains(line, "(") && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "/*") {
					result = append(result, trimmed)
				}
			}
		}
	}
	return result
}

func sigPatternsForExt(ext string) (patterns []*regexp.Regexp, cMode bool, fallback bool) {
	switch ext {
	case ".go":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^func `),
			regexp.MustCompile(`^type .*struct`),
			regexp.MustCompile(`^type .*interface`),
			regexp.MustCompile(`^var `),
			regexp.MustCompile(`^const `),
		}
	case ".py":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^class `),
			regexp.MustCompile(`^def `),
			regexp.MustCompile(`^async def `),
		}
	case ".js", ".ts", ".jsx", ".tsx":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^export `),
			regexp.MustCompile(`^function `),
			regexp.MustCompile(`^class `),
			regexp.MustCompile(`^interface `),
			regexp.MustCompile(`^type `),
			regexp.MustCompile(`^const .*=>`),
			regexp.MustCompile(`^async function `),
		}
	case ".rs":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^pub fn `),
			regexp.MustCompile(`^fn `),
			regexp.MustCompile(`^pub struct `),
			regexp.MustCompile(`^struct `),
			regexp.MustCompile(`^enum `),
			regexp.MustCompile(`^pub enum `),
			regexp.MustCompile(`^trait `),
			regexp.MustCompile(`^impl `),
		}
	case ".java", ".kt":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^public `),
			regexp.MustCompile(`^private `),
			regexp.MustCompile(`^protected `),
			regexp.MustCompile(`^class `),
			regexp.MustCompile(`^interface `),
			regexp.MustCompile(`^enum `),
			regexp.MustCompile(`^abstract `),
		}
	case ".rb":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^class `),
			regexp.MustCompile(`^module `),
			regexp.MustCompile(`^def `),
		}
	case ".php":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^class `),
			regexp.MustCompile(`^function `),
			regexp.MustCompile(`^interface `),
			regexp.MustCompile(`^trait `),
			regexp.MustCompile(`^public function `),
			regexp.MustCompile(`^private function `),
			regexp.MustCompile(`^protected function `),
		}
	case ".c", ".h", ".cpp", ".hpp":
		cMode = true
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^typedef `),
			regexp.MustCompile(`^struct `),
			regexp.MustCompile(`^enum `),
		}
	case ".sql":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`(?i)^CREATE `),
			regexp.MustCompile(`(?i)^ALTER `),
			regexp.MustCompile(`(?i)^DROP `),
		}
	case ".sh", ".bash":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^function `),
			regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\(\)`),
		}
	default:
		fallback = true
	}
	return
}

func estimateTokens(content []byte) int {
	return len(content) / 4
}

func getGitModTime(rootDir, relPath string) string {
	cmd := exec.Command("git", "log", "-1", "--format=%cr", "--", relPath)
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	result := strings.TrimSpace(string(out))
	return result
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

func isText(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}
