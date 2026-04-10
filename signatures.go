package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

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
