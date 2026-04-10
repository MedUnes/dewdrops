package main

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type fileChange struct {
	Status  string // "M", "A", "D", "R"
	Path    string // current path (for R: new path)
	OldPath string // only set for renames
}

func getGitModTime(rootDir, relPath string) string {
	cmd := exec.Command("git", "log", "-1", "--format=%cr", "--", relPath)
	cmd.Dir = rootDir
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

func gitShortHash(rootDir, ref string) string {
	cmd := exec.Command("git", "rev-parse", "--short", ref)
	cmd.Dir = rootDir
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ref
	}
	return strings.TrimSpace(string(out))
}

func gitDiff(rootDir, ref string) string {
	cmd := exec.Command("git", "diff", ref, "HEAD")
	cmd.Dir = rootDir
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func gitDiffNameStatus(rootDir, ref string) []fileChange {
	cmd := exec.Command("git", "diff", "--name-status", ref, "HEAD")
	cmd.Dir = rootDir
	cmd.Stderr = nil
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
