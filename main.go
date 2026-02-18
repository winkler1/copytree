package main

import (
	"bufio"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/bmatcuk/doublestar/v4"
)

//go:embed copytree.example.toml
var exampleConfig []byte

type Config struct {
	Source      string   `toml:"source"`
	Destination string   `toml:"destination"`
	Globs       []string `toml:"globs"`
}

func main() {
	force := flag.Bool("force", false, "overwrite even if target is newer")
	verbose := flag.Bool("v", false, "verbose output")
	configFile := flag.String("config", "copytree.toml", "config file path")
	flag.Parse()

	cfg, err := loadConfig(*configFile)
	if err != nil {
		if os.IsNotExist(err) && *configFile == "copytree.toml" {
			if offerCreateConfig(*configFile) {
				fmt.Printf("Created %s - edit it and run again\n", *configFile)
				os.Exit(0)
			}
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := copyTree(cfg, *force, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	if cfg.Source == "" {
		return nil, fmt.Errorf("source is required in config")
	}
	if cfg.Destination == "" {
		return nil, fmt.Errorf("destination is required in config")
	}
	if len(cfg.Globs) == 0 {
		cfg.Globs = []string{"**/*"}
	}
	return &cfg, nil
}

func copyTree(cfg *Config, force, verbose bool) error {
	fsys := os.DirFS(cfg.Source)
	for _, pattern := range cfg.Globs {
		matches, err := doublestar.Glob(fsys, pattern)
		if err != nil {
			return fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}

		for _, relPath := range matches {
			srcPath := filepath.Join(cfg.Source, relPath)
			srcInfo, err := os.Stat(srcPath)
			if err != nil {
				return err
			}
			if srcInfo.IsDir() {
				continue
			}

			dstPath := filepath.Join(cfg.Destination, relPath)

			copied, err := copyFile(srcPath, dstPath, srcInfo, force, verbose)
			if err != nil {
				return err
			}
			if verbose && copied {
				fmt.Printf("copied: %s -> %s\n", srcPath, dstPath)
			}
		}
	}
	return nil
}

func copyFile(src, dst string, srcInfo os.FileInfo, force, verbose bool) (bool, error) {
	dstInfo, err := os.Stat(dst)
	if err == nil {
		if dstInfo.ModTime().After(srcInfo.ModTime()) {
			if !force {
				if verbose {
					fmt.Printf("skipped (target newer): %s\n", dst)
				}
				return false, nil
			}
		} else if !srcInfo.ModTime().After(dstInfo.ModTime()) {
			if verbose {
				fmt.Printf("skipped (same time): %s\n", dst)
			}
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return false, err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return false, err
	}

	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return false, err
	}

	return true, nil
}

func offerCreateConfig(path string) bool {
	fmt.Printf("No %s found. Create one? [y/N] ", path)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(answer)) != "y" {
		return false
	}
	content := strings.TrimPrefix(string(exampleConfig), "# Example copytree configuration\n\n")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create config: %v\n", err)
		return false
	}
	return true
}
