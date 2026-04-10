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
