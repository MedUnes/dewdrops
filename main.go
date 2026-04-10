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
	"sync"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

var version = "dev"

const DefaultOutputFileName = "dewdrops_context.md"
const warnTokenThreshold = 100_000

// RunOptions configures the behavior of Run.
type RunOptions struct {
	MapMode    bool
	MapFilter  string // "" = supported exts only, "any" = all textual, "go,py" = specific
	FromPaths  []string
	OutputFile string
	SinceRef   string
}

// mapFlagValue implements flag.Value with IsBoolFlag so --map works without a value
// and --map=go,py works with a value.
type mapFlagValue struct {
	enabled bool
	filter  string
}

func (f *mapFlagValue) String() string   { return f.filter }
func (f *mapFlagValue) IsBoolFlag() bool { return true }
func (f *mapFlagValue) Set(s string) error {
	f.enabled = true
	if s == "true" {
		f.filter = ""
	} else {
		f.filter = s
	}
	return nil
}

// supportedMapExts are extensions with real signature extraction patterns.
// Config files (.yaml, .toml, .json, .env) are excluded — their fallback
// (first 3 lines) produces noise. Include explicitly via --map=yaml,json.
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

func main() {
	var mapVal mapFlagValue
	flag.Var(&mapVal, "map", "Output structural map (use --map=go,py to filter by extension, --map=any for all text files)")
	fromFlag := flag.String("from", "", "Comma-separated list of file/dir paths to include")
	sinceFlag := flag.String("since", "", "Git ref to diff against HEAD (branch, tag, hash, HEAD~N)")
	outputFlag := flag.String("o", "", "Output file path (default: dewdrops_context.md)")
	versionFlag := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: dewdrops [options] <repo-path>

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
  dewdrops --from internal/auth/ .                  # Dump specific directory
  dewdrops --map --from internal/auth/,cmd/ .       # Map of specific subtree
  dewdrops --since main .                           # Review changes vs main
  dewdrops --since HEAD~3 -o review.md .            # Last 3 commits, custom path
`)
	}
	flag.Parse()

	if *versionFlag {
		fmt.Printf("dewdrops %s\n", version)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: Missing required argument <repo-path>")
		flag.Usage()
		os.Exit(1)
	}

	rootDir := flag.Arg(0)
	opts := RunOptions{
		MapMode:    mapVal.enabled,
		MapFilter:  mapVal.filter,
		OutputFile: DefaultOutputFileName,
	}
	if *outputFlag != "" {
		opts.OutputFile = *outputFlag
	}
	if *sinceFlag != "" {
		opts.SinceRef = *sinceFlag
		if *outputFlag == "" {
			opts.OutputFile = sinceOutputFileName(*sinceFlag)
		}
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

	if opts.SinceRef != "" && (opts.MapMode || len(opts.FromPaths) > 0) {
		return fmt.Errorf("--since cannot be combined with --map or --from.\n   --since produces its own composite output (map + diff + content).")
	}

	fmt.Printf("dewdrops: Scanning '%s'💧💧💧\n", rootDir)

	if opts.SinceRef != "" {
		outFile, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer outFile.Close()
		writer := bufio.NewWriter(outFile)

		stats, err := writeSinceOutput(writer, rootDir, opts.SinceRef)
		if err != nil {
			return err
		}
		writer.Flush()

		fi, _ := outFile.Stat()
		fileSizeMB := float64(fi.Size()) / 1024 / 1024

		if stats.filesChanged > 0 {
			shortHead := gitShortHash(rootDir, "HEAD")
			shortRef := gitShortHash(rootDir, opts.SinceRef)
			fmt.Println("\n------------------------------------------------")
			fmt.Printf("Since Summary (%s vs %s)\n", shortRef, shortHead)
			fmt.Println("------------------------------------------------")
			fmt.Printf("Files Changed       : %d (%d modified, %d added, %d deleted)\n",
				stats.filesChanged, stats.filesModified, stats.filesAdded, stats.filesDeleted)
			fmt.Printf("Signatures Extracted: %d\n", stats.sigCount)
			fmt.Printf("Estimated Tokens    : %s\n", formatNumber(stats.totalTokens))
			fmt.Printf("Output Size         : %.3f MB (%s)\n", fileSizeMB, outputFile)
			fmt.Println("------------------------------------------------")
		}

		if fi, err := outFile.Stat(); err == nil {
			estimatedTokens := fi.Size() / 4
			if estimatedTokens > warnTokenThreshold {
				fmt.Fprintf(os.Stderr, "\n⚠️  Output is ~%s tokens. This may exceed your LLM's context window.\n", formatNumber(int(estimatedTokens)))
				fmt.Fprintln(os.Stderr, "    Consider using --map first, then --from with specific files.")
			}
		}

		return nil
	}

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
		stats := writeMapOutput(writer, rootDir, filePaths, opts.MapFilter)
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

	if fi, err := outFile.Stat(); err == nil {
		estimatedTokens := fi.Size() / 4
		if estimatedTokens > warnTokenThreshold {
			fmt.Fprintf(os.Stderr, "\n⚠️  Output is ~%s tokens. This may exceed your LLM's context window.\n", formatNumber(int(estimatedTokens)))
			fmt.Fprintln(os.Stderr, "    Consider using --map first, then --from with specific files.")
		}
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

var (
	goSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^func `),
		regexp.MustCompile(`^type .*struct`),
		regexp.MustCompile(`^type .*interface`),
		regexp.MustCompile(`^var `),
		regexp.MustCompile(`^const `),
	}
	pySigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^class `),
		regexp.MustCompile(`^def `),
		regexp.MustCompile(`^async def `),
	}
	jsSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*export `),
		regexp.MustCompile(`^\s*function `),
		regexp.MustCompile(`^\s*class `),
		regexp.MustCompile(`^\s*interface `),
		regexp.MustCompile(`^\s*type `),
		regexp.MustCompile(`^\s*const .*=>`),
		regexp.MustCompile(`^\s*async function `),
	}
	rsSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*pub fn `),
		regexp.MustCompile(`^\s*fn `),
		regexp.MustCompile(`^\s*pub struct `),
		regexp.MustCompile(`^\s*struct `),
		regexp.MustCompile(`^\s*enum `),
		regexp.MustCompile(`^\s*pub enum `),
		regexp.MustCompile(`^\s*trait `),
		regexp.MustCompile(`^\s*impl `),
	}
	javaSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*public `),
		regexp.MustCompile(`^\s*private `),
		regexp.MustCompile(`^\s*protected `),
		regexp.MustCompile(`^\s*class `),
		regexp.MustCompile(`^\s*interface `),
		regexp.MustCompile(`^\s*enum `),
		regexp.MustCompile(`^\s*abstract `),
	}
	rbSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*class `),
		regexp.MustCompile(`^\s*module `),
		regexp.MustCompile(`^\s*def `),
	}
	phpSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*class `),
		regexp.MustCompile(`^\s*function `),
		regexp.MustCompile(`^\s*interface `),
		regexp.MustCompile(`^\s*trait `),
		regexp.MustCompile(`^\s*public function `),
		regexp.MustCompile(`^\s*private function `),
		regexp.MustCompile(`^\s*protected function `),
	}
	cSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^typedef `),
		regexp.MustCompile(`^struct `),
		regexp.MustCompile(`^enum `),
	}
	sqlSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^CREATE `),
		regexp.MustCompile(`(?i)^ALTER `),
		regexp.MustCompile(`(?i)^DROP `),
	}
	shSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^function `),
		regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\(\)`),
	}
	mdSigPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^#{1,6} `),
	}
)

func sigPatternsForExt(ext string) (patterns []*regexp.Regexp, cMode bool, fallback bool) {
	switch ext {
	case ".go":
		patterns = goSigPatterns
	case ".py":
		patterns = pySigPatterns
	case ".js", ".ts", ".jsx", ".tsx":
		patterns = jsSigPatterns
	case ".rs":
		patterns = rsSigPatterns
	case ".java", ".kt":
		patterns = javaSigPatterns
	case ".rb":
		patterns = rbSigPatterns
	case ".php":
		patterns = phpSigPatterns
	case ".c", ".h", ".cpp", ".hpp":
		cMode = true
		patterns = cSigPatterns
	case ".sql":
		patterns = sqlSigPatterns
	case ".sh", ".bash":
		patterns = shSigPatterns
	case ".md", ".markdown":
		patterns = mdSigPatterns
	default:
		fallback = true
	}
	return
}

type fileChange struct {
	Status  string // "M", "A", "D", "R"
	Path    string // current path (for R: new path)
	OldPath string // only set for renames
}

type sinceStats struct {
	filesChanged  int
	filesAdded    int
	filesModified int
	filesDeleted  int
	totalTokens   int
	sigCount      int
}

func gitShortHash(rootDir, ref string) string {
	cmd := exec.Command("git", "rev-parse", "--short", ref)
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return ref
	}
	return strings.TrimSpace(string(out))
}

func gitDiff(rootDir, ref string) string {
	cmd := exec.Command("git", "diff", ref, "HEAD")
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func gitDiffNameStatus(rootDir, ref string) []fileChange {
	cmd := exec.Command("git", "diff", "--name-status", ref, "HEAD")
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var changes []fileChange
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		fc := fileChange{}
		if strings.HasPrefix(status, "R") {
			fc.Status = "R"
			fc.OldPath = parts[1]
			if len(parts) >= 3 {
				fc.Path = parts[2]
			}
		} else {
			fc.Status = status
			fc.Path = parts[1]
		}
		changes = append(changes, fc)
	}
	return changes
}

func sinceOutputFileName(ref string) string {
	safe := strings.NewReplacer("~", "_", "^", "_", "/", "_", "\\", "_", "..", "_").Replace(ref)
	return fmt.Sprintf("dewdrops_since_%s.md", safe)
}

func writeSinceOutput(writer *bufio.Writer, rootDir string, ref string) (sinceStats, error) {
	var stats sinceStats

	cmd := exec.Command("git", "rev-parse", "--short", ref)
	cmd.Dir = rootDir
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
	writer.WriteString(formatSinceTreeOutput(sinceLines))
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

func buildSinceTree(changes []fileChange, tokenMap map[string]int, statusMap map[string]string) *treeNode {
	root := &treeNode{isDir: true}
	for _, c := range changes {
		parts := strings.Split(c.Path, "/")
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			var child *treeNode
			for _, ch := range current.children {
				if ch.name == part {
					child = ch
					break
				}
			}
			if child == nil {
				child = &treeNode{
					name:  part,
					isDir: !isLast,
				}
				if isLast {
					child.tokens = tokenMap[c.Path]
					child.modTime = statusMap[c.Path] // repurpose modTime for status
				}
				current.children = append(current.children, child)
			}
			current = child
		}
	}
	aggregateTokens(root)
	return root
}

func formatSinceTreeOutput(lines []treeLine) string {
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

		status := l.modTime // modTime holds the status for since mode
		if status == "D" {
			sb.WriteString(fmt.Sprintf("(deleted)     [%s]", status))
		} else if !l.isDir {
			sb.WriteString(fmt.Sprintf("(~%s tok)  [%s]", formatNumber(l.tokens), status))
		} else {
			sb.WriteString(fmt.Sprintf("(~%s tok)", formatNumber(l.tokens)))
		}
		sb.WriteString("\n")
	}
	return sb.String()
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

func batchGitModTimes(rootDir string, filePaths []string) map[string]string {
	result := make(map[string]string)
	var mu sync.Mutex
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup

	for _, relPath := range filePaths {
		wg.Add(1)
		sem <- struct{}{}
		go func(rp string) {
			defer wg.Done()
			defer func() { <-sem }()
			modTime := getGitModTime(rootDir, rp)
			if modTime != "" {
				mu.Lock()
				result[rp] = modTime
				mu.Unlock()
			}
		}(relPath)
	}
	wg.Wait()
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
