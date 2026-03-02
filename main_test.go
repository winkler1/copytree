package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "copytree.toml")

	t.Run("valid config", func(t *testing.T) {
		content := `source = "/src"
destination = "/dst"
globs = ["*.txt", "subdir/*.go"]`
		err := os.WriteFile(configPath, []byte(content), 0644)
		assert.NoError(t, err)

		cfg, err := loadConfig(configPath)
		assert.NoError(t, err)
		assert.Equal(t, "/src", cfg.Source)
		assert.Equal(t, "/dst", cfg.Destination)
		assert.Equal(t, []string{"*.txt", "subdir/*.go"}, cfg.Globs)
	})

	t.Run("missing source", func(t *testing.T) {
		content := `destination = "/dst"`
		err := os.WriteFile(configPath, []byte(content), 0644)
		assert.NoError(t, err)

		_, err = loadConfig(configPath)
		assert.ErrorContains(t, err, "source is required")
	})

	t.Run("missing destination", func(t *testing.T) {
		content := `source = "/src"`
		err := os.WriteFile(configPath, []byte(content), 0644)
		assert.NoError(t, err)

		_, err = loadConfig(configPath)
		assert.ErrorContains(t, err, "destination is required")
	})

	t.Run("empty globs errors", func(t *testing.T) {
		content := `source = "/src"
destination = "/dst"`
		err := os.WriteFile(configPath, []byte(content), 0644)
		assert.NoError(t, err)

		_, err = loadConfig(configPath)
		assert.ErrorContains(t, err, "globs is empty")
	})
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")

	t.Run("copies new file", func(t *testing.T) {
		err := os.WriteFile(srcPath, []byte("hello"), 0644)
		assert.NoError(t, err)

		srcInfo, _ := os.Stat(srcPath)
		copied, err := copyFile(srcPath, dstPath, srcInfo, false, false)
		assert.NoError(t, err)
		assert.True(t, copied)

		content, _ := os.ReadFile(dstPath)
		assert.Equal(t, "hello", string(content))
	})

	t.Run("skips when target newer", func(t *testing.T) {
		os.Remove(dstPath)
		err := os.WriteFile(srcPath, []byte("old"), 0644)
		assert.NoError(t, err)
		oldTime := time.Now().Add(-time.Hour)
		os.Chtimes(srcPath, oldTime, oldTime)

		err = os.WriteFile(dstPath, []byte("new"), 0644)
		assert.NoError(t, err)

		srcInfo, _ := os.Stat(srcPath)
		copied, err := copyFile(srcPath, dstPath, srcInfo, false, false)
		assert.NoError(t, err)
		assert.False(t, copied)

		content, _ := os.ReadFile(dstPath)
		assert.Equal(t, "new", string(content))
	})

	t.Run("force overwrites newer target", func(t *testing.T) {
		os.Remove(dstPath)
		err := os.WriteFile(srcPath, []byte("forced"), 0644)
		assert.NoError(t, err)
		oldTime := time.Now().Add(-time.Hour)
		os.Chtimes(srcPath, oldTime, oldTime)

		err = os.WriteFile(dstPath, []byte("existing"), 0644)
		assert.NoError(t, err)

		srcInfo, _ := os.Stat(srcPath)
		copied, err := copyFile(srcPath, dstPath, srcInfo, true, false)
		assert.NoError(t, err)
		assert.True(t, copied)

		content, _ := os.ReadFile(dstPath)
		assert.Equal(t, "forced", string(content))
	})
}

func TestCopyTree(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	subdir := filepath.Join(srcDir, "subdir")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("one"), 0644)
	os.WriteFile(filepath.Join(subdir, "file2.txt"), []byte("two"), 0644)

	cfg := &Config{
		Source:      srcDir,
		Destination: dstDir,
		Globs:       []string{"**/*.txt"},
	}

	err := copyTree(cfg, false, false)
	assert.NoError(t, err)

	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "one", string(content1))

	content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "two", string(content2))
}
