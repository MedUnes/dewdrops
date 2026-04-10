package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RunOptions struct {
	MapMode    bool
	MapFilter  string
	FromPaths  []string
	OutputFile string
	SinceRef   string
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

	fmt.Fprintf(os.Stderr, "dewdrops: Scanning '%s' 💧💧💧\n", rootDir)

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

		checkOutputSize(outputFile)
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

	checkOutputSize(outputFile)
	return nil
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

func checkOutputSize(outputFile string) {
	fi, err := os.Stat(outputFile)
	if err != nil {
		return
	}
	estimatedTokens := fi.Size() / 4
	if estimatedTokens > warnTokenThreshold {
		fmt.Fprintf(os.Stderr, "\n⚠️  Output is ~%s tokens. This may exceed your LLM's context window.\n", formatNumber(int(estimatedTokens)))
		fmt.Fprintln(os.Stderr, "    Consider using --map first, then --from with specific files.")
	}
}
