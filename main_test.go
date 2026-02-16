package main

import (
	"os"
	"path/filepath"
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

	err = Run(tempDir, outputFile)
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
