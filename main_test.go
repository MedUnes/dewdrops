package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_Smoke(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "dewdrops_test_output.md")

	mustIncludePath := filepath.Join(tempDir, "main.go")
	err := os.WriteFile(mustIncludePath, []byte("package main\nfunc main() {}"), 0644)
	require.NoError(t, err, "Setup: Failed to create main.go")

	subDir := filepath.Join(tempDir, "pkg")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err, "Setup: Failed to create pkg directory")

	subFilePath := filepath.Join(subDir, "helper.go")
	err = os.WriteFile(subFilePath, []byte("package pkg"), 0644)
	require.NoError(t, err, "Setup: Failed to create pkg/helper.go")

	ignoredPath := filepath.Join(tempDir, "secret.env")
	err = os.WriteFile(ignoredPath, []byte("API_KEY=12345"), 0644)
	require.NoError(t, err, "Setup: Failed to create secret.env")

	gitignorePath := filepath.Join(tempDir, ".gitignore")
	err = os.WriteFile(gitignorePath, []byte("*.env\n"), 0644)
	require.NoError(t, err, "Setup: Failed to create .gitignore")

	err = Run(tempDir, RunOptions{OutputFile: outputFile})
	require.NoError(t, err, "Run() execution failed")

	contentBytes, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Failed to read output file")
	content := string(contentBytes)

	t.Run("Should include valid source files", func(t *testing.T) {
		assert.Contains(t, content, "### file: main.go")
		assert.Contains(t, content, "### file: pkg/helper.go")
	})

	t.Run("Should strictly ignore .env secrets", func(t *testing.T) {
		assert.NotContains(t, content, "secret.env", "Filename should be hidden")
		assert.NotContains(t, content, "API_KEY=12345", "Content should be hidden")
	})

	t.Run("Should apply Markdown formatting", func(t *testing.T) {
		assert.Contains(t, content, "# Repository Context", "Header missing")
		assert.Contains(t, content, "```go", "Go syntax highlighting missing")
	})

	t.Run("Should generate directory tree", func(t *testing.T) {
		assert.Contains(t, content, "├── main.go", "Tree structure missing")
	})
}

// --- Test fixture setup ---

// setupFixtureRepo creates a temp git repo with the spec's fixture structure.
// Returns the repo root path and an output file path.
func setupFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	outputFile := filepath.Join(root, "dewdrops_test_output.md")

	files := map[string]string{
		"main.go": "package main\n\nfunc main() {}\nfunc helper(x int) string { return \"\" }\n",
		"internal/auth/jwt.go": "package auth\n\ntype Claims struct {\n\tUserID string\n}\n\nfunc NewToken(user string) (string, error) { return \"\", nil }\nfunc ValidateToken(raw string) (*Claims, error) { return nil, nil }\n",
		"internal/auth/middleware.go": "package auth\n\nfunc RequireAuth(next int) int { return next }\n",
		"internal/store/db.go":        "package store\n\ntype Store struct {\n\tDSN string\n}\n\nfunc NewStore(dsn string) *Store { return nil }\nfunc (s *Store) GetUser(id string) string { return \"\" }\n",
		"README.md":                   "# Test Project\n\nThis is a test.\n",
		".gitignore":                  "*.log\ntmp/\n",
	}

	// Create all tracked files
	for path, content := range files {
		fullPath := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	// Create gitignored files
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tmp"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tmp/ignored.txt"), []byte("should not appear"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "debug.log"), []byte("should not appear"), 0644))

	// Initialize git repo
	gitInit(t, root)

	return root, outputFile
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command failed: %s\n%s", strings.Join(args, " "), string(out))
	}
}

func runAndRead(t *testing.T, root string, outputFile string, opts RunOptions) string {
	t.Helper()
	opts.OutputFile = outputFile
	err := Run(root, opts)
	require.NoError(t, err)
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	return string(content)
}

// --- Tests for --map ---

func TestMapOutputContainsTree(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.Contains(t, content, "# Repository Map:")
	assert.Contains(t, content, "## Structure")
	assert.Contains(t, content, "main.go")
	assert.Contains(t, content, "internal/auth/jwt.go")
	assert.NotContains(t, content, "tmp/ignored.txt")
	assert.NotContains(t, content, "debug.log")

	// Tree should be wrapped in a fenced code block
	structIdx := strings.Index(content, "## Structure")
	sigIdx := strings.Index(content, "## Signatures")
	treePortion := content[structIdx:sigIdx]
	assert.Contains(t, treePortion, "```text")
	assert.Contains(t, treePortion, "```\n")
}

func TestMapOutputContainsSignatures(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.Contains(t, content, "## Signatures")
	assert.Contains(t, content, "func main()")
	assert.Contains(t, content, "func helper(x int) string")
	assert.Contains(t, content, "type Claims struct")
	assert.Contains(t, content, "func NewToken(user string) (string, error)")
	assert.Contains(t, content, "func ValidateToken(raw string) (*Claims, error)")
	assert.Contains(t, content, "func (s *Store) GetUser(id string) string")
	assert.Contains(t, content, "type Store struct")
}

func TestMapOutputContainsTokenEstimates(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.Contains(t, content, "tok")
	assert.Contains(t, content, "tokens (estimated)")
}

func TestMapOutputDoesNotContainFileContents(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.NotContains(t, content, "```go\npackage main")
	assert.NotContains(t, content, "### file:")
}

func TestMapSignatureExtractionPython(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	pyContent := `class MyClass:
    def method(self, x):
        pass

def standalone(a, b):
    return a + b

async def fetch_data(url):
    pass
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "script.py"), []byte(pyContent), 0644))
	cmd := exec.Command("git", "add", "script.py")
	cmd.Dir = root
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "add python")
	cmd.Dir = root
	require.NoError(t, cmd.Run())

	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.Contains(t, content, "class MyClass:")
	assert.Contains(t, content, "def standalone(a, b):")
	assert.Contains(t, content, "async def fetch_data(url):")
	assert.NotContains(t, content, "def method(self, x):")
}

func TestMapFallbackSignatures(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	txtContent := "First line\nSecond line\nThird line\nFourth line\nFifth line\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte(txtContent), 0644))
	cmd := exec.Command("git", "add", "notes.txt")
	cmd.Dir = root
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "add notes")
	cmd.Dir = root
	require.NoError(t, cmd.Run())

	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.Contains(t, content, "First line")
	assert.Contains(t, content, "Second line")
	assert.Contains(t, content, "Third line")
}

func TestMapSignatureExtractionMarkdown(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	mdContent := "# My Project\n\nSome intro text.\n\n## Installation\n\nRun this.\n\n## Usage\n\nDo that.\n\n### Advanced\n\nMore stuff.\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs.md"), []byte(mdContent), 0644))
	cmd := exec.Command("git", "add", "docs.md")
	cmd.Dir = root
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "add docs")
	cmd.Dir = root
	require.NoError(t, cmd.Run())

	content := runAndRead(t, root, outFile, RunOptions{MapMode: true})

	assert.Contains(t, content, "# My Project")
	assert.Contains(t, content, "## Installation")
	assert.Contains(t, content, "## Usage")
	assert.Contains(t, content, "### Advanced")
	assert.NotContains(t, content, "Some intro text.")
	assert.NotContains(t, content, "Run this.")
}

// --- Tests for --from ---

func TestFromSingleFile(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{FromPaths: []string{"main.go"}})

	assert.Contains(t, content, "### file: main.go")
	assert.Contains(t, content, "package main")
	assert.NotContains(t, content, "### file: internal/auth/jwt.go")
	assert.NotContains(t, content, "### file: README.md")
}

func TestFromMultipleFiles(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{FromPaths: []string{"internal/auth/jwt.go", "internal/auth/middleware.go"}})

	assert.Contains(t, content, "### file: internal/auth/jwt.go")
	assert.Contains(t, content, "### file: internal/auth/middleware.go")
	assert.NotContains(t, content, "### file: main.go")
}

func TestFromDirectory(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{FromPaths: []string{"internal/auth/"}})

	assert.Contains(t, content, "### file: internal/auth/jwt.go")
	assert.Contains(t, content, "### file: internal/auth/middleware.go")
	assert.NotContains(t, content, "### file: internal/store/db.go")
}

func TestFromNonExistentPath(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	// Run manually to capture stderr
	opts := RunOptions{
		OutputFile: outFile,
		FromPaths:  []string{"nonexistent.go", "main.go"},
	}

	// Redirect stderr to capture warnings
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Run(root, opts)

	w.Close()
	stderrBytes := make([]byte, 4096)
	n, _ := r.Read(stderrBytes)
	os.Stderr = oldStderr

	require.NoError(t, err)
	stderrOutput := string(stderrBytes[:n])
	assert.Contains(t, stderrOutput, "Skipped (not found or ignored): nonexistent.go")

	content, _ := os.ReadFile(outFile)
	assert.Contains(t, string(content), "### file: main.go")
}

func TestFromAllPathsInvalid(t *testing.T) {
	root, _ := setupFixtureRepo(t)
	outFile := filepath.Join(root, "out.md")

	opts := RunOptions{
		OutputFile: outFile,
		FromPaths:  []string{"nonexistent.go", "also_fake.go"},
	}

	// Suppress stderr warnings
	oldStderr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	err := Run(root, opts)
	os.Stderr = oldStderr

	require.Error(t, err)
	assert.Contains(t, err.Error(), "No valid files found")
}

func TestFromRespectsGitignore(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	opts := RunOptions{
		OutputFile: outFile,
		FromPaths:  []string{"tmp/ignored.txt", "main.go"},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Run(root, opts)

	w.Close()
	stderrBytes := make([]byte, 4096)
	n, _ := r.Read(stderrBytes)
	os.Stderr = oldStderr

	require.NoError(t, err)
	stderrOutput := string(stderrBytes[:n])
	assert.Contains(t, stderrOutput, "Skipped (not found or ignored): tmp/ignored.txt")

	content, _ := os.ReadFile(outFile)
	assert.Contains(t, string(content), "### file: main.go")
}

func TestFromTreeIsScoped(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{FromPaths: []string{"internal/auth/"}})

	assert.Contains(t, content, "internal/auth/")
	assert.NotContains(t, content, "internal/store/")
	assert.NotContains(t, content, "├── main.go")
}

// --- Tests for combinations ---

func TestMapWithFrom(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{MapMode: true, FromPaths: []string{"internal/auth/"}})

	assert.Contains(t, content, "# Repository Map:")
	// Signatures from auth files
	assert.Contains(t, content, "func NewToken(user string) (string, error)")
	assert.Contains(t, content, "func RequireAuth(next int) int")
	// No signatures from store
	assert.NotContains(t, content, "func NewStore")
	// No full file contents
	assert.NotContains(t, content, "### file:")
}

func TestDefaultBehaviorUnchanged(t *testing.T) {
	root, outFile := setupFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{})

	assert.Contains(t, content, "### file: main.go")
	assert.Contains(t, content, "package main")
	assert.Contains(t, content, "### file: internal/auth/jwt.go")
	assert.Contains(t, content, "### file: README.md")
	assert.Contains(t, content, "# Test Project")
}

// --- Tests for oversize warning ---

func TestLargeOutputWarning(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	// Create a 500KB file
	bigContent := strings.Repeat("x", 500*1024)
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.txt"), []byte(bigContent), 0644))
	cmd := exec.Command("git", "add", "big.txt")
	cmd.Dir = root
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "add big file")
	cmd.Dir = root
	require.NoError(t, cmd.Run())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Run(root, RunOptions{OutputFile: outFile, FromPaths: []string{"big.txt"}})

	w.Close()
	stderrBytes := make([]byte, 8192)
	n, _ := r.Read(stderrBytes)
	os.Stderr = oldStderr

	require.NoError(t, err)
	assert.Contains(t, string(stderrBytes[:n]), "may exceed your LLM's context window")
}

func TestSmallOutputNoWarning(t *testing.T) {
	root, outFile := setupFixtureRepo(t)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Run(root, RunOptions{OutputFile: outFile})

	w.Close()
	stderrBytes := make([]byte, 8192)
	n, _ := r.Read(stderrBytes)
	os.Stderr = oldStderr

	require.NoError(t, err)
	assert.NotContains(t, string(stderrBytes[:n]), "may exceed")
}

// --- Tests for -o flag ---

func TestCustomOutputPath(t *testing.T) {
	root, _ := setupFixtureRepo(t)
	customPath := filepath.Join(root, "subdir", "custom_output.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(customPath), 0755))

	err := Run(root, RunOptions{OutputFile: customPath})
	require.NoError(t, err)

	content, err := os.ReadFile(customPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "### file: main.go")
}

func TestDefaultOutputPathUnchanged(t *testing.T) {
	root, _ := setupFixtureRepo(t)

	err := Run(root, RunOptions{OutputFile: filepath.Join(root, DefaultOutputFileName)})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(root, DefaultOutputFileName))
	assert.NoError(t, err, "default output file should exist")
}

// --- Tests for batch git mod-times ---

func TestBatchGitModTimesReturnsResults(t *testing.T) {
	root, _ := setupFixtureRepo(t)
	result := batchGitModTimes(root, []string{"main.go", "internal/auth/jwt.go"})

	assert.NotEmpty(t, result["main.go"])
	assert.NotEmpty(t, result["internal/auth/jwt.go"])
	// Git relative format contains time words
	for _, v := range result {
		assert.True(t, strings.Contains(v, "ago") || strings.Contains(v, "second") || strings.Contains(v, "minute"),
			"expected git relative time format, got: %s", v)
	}
}

func TestBatchGitModTimesHandlesUntracked(t *testing.T) {
	root, _ := setupFixtureRepo(t)

	// Create an untracked file (not committed)
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked.go"), []byte("package main\n"), 0644))

	result := batchGitModTimes(root, []string{"untracked.go", "main.go"})

	// Untracked file should be absent or empty
	assert.Empty(t, result["untracked.go"])
	// Tracked file should still work
	assert.NotEmpty(t, result["main.go"])
}

// --- Tests for --since ---

func setupSinceFixtureRepo(t *testing.T) (root string, outputFile string, baseRef string) {
	t.Helper()
	root = t.TempDir()
	outputFile = filepath.Join(root, "out.md")

	// First commit: base state
	require.NoError(t, os.MkdirAll(filepath.Join(root, "internal/auth"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "internal/auth/jwt.go"),
		[]byte("package auth\n\nfunc OldFunc() {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "internal/auth/old.go"),
		[]byte("package auth\n\nfunc Deprecated() {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0644))
	gitInit(t, root)

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, _ := cmd.Output()
	baseRef = strings.TrimSpace(string(out))

	// Second commit: modify jwt.go, add new.go, delete old.go
	require.NoError(t, os.WriteFile(filepath.Join(root, "internal/auth/jwt.go"),
		[]byte("package auth\n\nfunc OldFunc() {}\nfunc NewFunc(x int) error { return nil }\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "internal/auth/new.go"),
		[]byte("package auth\n\nfunc BrandNew() string { return \"\" }\n"), 0644))
	require.NoError(t, os.Remove(filepath.Join(root, "internal/auth/old.go")))

	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = root
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "second commit")
	cmd.Dir = root
	require.NoError(t, cmd.Run())

	return root, outputFile, baseRef
}

func TestSinceOutputContainsAllSections(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "# Changes:")
	assert.Contains(t, content, "## Map of Changed Files")
	assert.Contains(t, content, "## Diff")
	assert.Contains(t, content, "## File Contents")
}

func TestSinceOutputContainsDiff(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "```diff")
	assert.Contains(t, content, "+func NewFunc(x int) error")
}

func TestSinceOutputContainsModifiedFileContent(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "### file: internal/auth/jwt.go [MODIFIED]")
	assert.Contains(t, content, "func NewFunc(x int) error")
}

func TestSinceOutputContainsAddedFile(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "### file: internal/auth/new.go [ADDED]")
	assert.Contains(t, content, "func BrandNew() string")
}

func TestSinceOutputExcludesDeletedFileContent(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "[D]")
	assert.NotContains(t, content, "### file: internal/auth/old.go")
	// Check func Deprecated doesn't appear in content section (it may appear in diff)
	contentSection := content[strings.Index(content, "## File Contents"):]
	assert.NotContains(t, contentSection, "func Deprecated()")
}

func TestSinceOutputExcludesUnchangedFiles(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.NotContains(t, content, "### file: main.go")
	// Tree should not contain main.go
	treeSection := content[strings.Index(content, "## Map"):strings.Index(content, "## Diff")]
	assert.NotContains(t, treeSection, "main.go")
}

func TestSinceOutputContainsSignatures(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "func OldFunc()")
	assert.Contains(t, content, "func NewFunc(x int) error")
	assert.Contains(t, content, "func BrandNew() string")
}

func TestSinceTreeShowsChangeStatus(t *testing.T) {
	root, outFile, baseRef := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: baseRef})

	assert.Contains(t, content, "[M]")
	assert.Contains(t, content, "[A]")
	assert.Contains(t, content, "[D]")
}

func TestSinceWithRelativeRef(t *testing.T) {
	root, outFile, _ := setupSinceFixtureRepo(t)
	content := runAndRead(t, root, outFile, RunOptions{SinceRef: "HEAD~1"})

	assert.Contains(t, content, "# Changes:")
	assert.Contains(t, content, "internal/auth/jwt.go")
}

func TestSinceOutputFilename(t *testing.T) {
	root, _, _ := setupSinceFixtureRepo(t)
	expectedFile := filepath.Join(root, "dewdrops_since_HEAD_1.md")

	// Clean up if exists
	os.Remove(expectedFile)

	err := Run(root, RunOptions{
		SinceRef:   "HEAD~1",
		OutputFile: filepath.Join(root, sinceOutputFileName("HEAD~1")),
	})
	require.NoError(t, err)

	_, err = os.Stat(expectedFile)
	assert.NoError(t, err, "expected auto-named output file to exist")
}

func TestSinceWithCustomOutput(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	customPath := filepath.Join(root, "custom_review.md")

	err := Run(root, RunOptions{SinceRef: baseRef, OutputFile: customPath})
	require.NoError(t, err)

	_, err = os.Stat(customPath)
	assert.NoError(t, err, "custom output file should exist")
}

func TestSinceInvalidRef(t *testing.T) {
	root, _, _ := setupSinceFixtureRepo(t)
	outFile := filepath.Join(root, "out.md")

	err := Run(root, RunOptions{SinceRef: "nonexistent_branch_xyz", OutputFile: outFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid git ref")
}

func TestSinceNoChanges(t *testing.T) {
	root, outFile, _ := setupSinceFixtureRepo(t)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := Run(root, RunOptions{SinceRef: "HEAD", OutputFile: outFile})

	w.Close()
	stderrBytes := make([]byte, 4096)
	n, _ := r.Read(stderrBytes)
	os.Stderr = oldStderr

	require.NoError(t, err)
	assert.Contains(t, string(stderrBytes[:n]), "No changes found")
}

func TestSinceMutualExclusivityWithMap(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	outFile := filepath.Join(root, "out.md")

	err := Run(root, RunOptions{SinceRef: baseRef, MapMode: true, OutputFile: outFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined")
}

func TestSinceMutualExclusivityWithFrom(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	outFile := filepath.Join(root, "out.md")

	err := Run(root, RunOptions{SinceRef: baseRef, FromPaths: []string{"main.go"}, OutputFile: outFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined")
}
