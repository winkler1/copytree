package main

import (
	"bufio"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
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
	watch := flag.Bool("watch", false, "watch config file for changes")
	configFile := flag.String("config", "copytree.toml", "config file path")
	flag.Parse()

	if *watch {
		if _, err := os.Stat(*configFile); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error: config file %s not found\n", *configFile)
			os.Exit(1)
		}
		watchConfig(*configFile, *force, *verbose)
		return
	}

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
		return nil, fmt.Errorf("globs is empty - nothing to copy")
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
			if isHidden(relPath) && !globTargetsHidden(pattern) {
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
	cleanDSStore(cfg.Destination, verbose)
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

func isHidden(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func globTargetsHidden(pattern string) bool {
	for _, part := range strings.Split(pattern, string(filepath.Separator)) {
		if part == "**" || part == "*" {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func cleanDSStore(root string, verbose bool) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == ".DS_Store" {
			if verbose {
				fmt.Printf("deleted: %s\n", path)
			}
			os.Remove(path)
		}
		return nil
	})
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

func watchConfig(configFile string, force, verbose bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	if err := watcher.Add(configFile); err != nil {
		log.Fatal(err)
	}

	runCopy := func() {
		fmt.Print("\033[2J\033[H")
		fmt.Printf("[%s] Config changed, running copy...\n\n", time.Now().Format("15:04:05"))
		defer fmt.Println("\nWatching for changes...")
		cfg, err := loadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
			return
		}
		fmt.Printf("Source:      %s\n", cfg.Source)
		fmt.Printf("Destination: %s\n", cfg.Destination)
		fmt.Printf("Globs:       %v\n\n", cfg.Globs)
		if err := copyTree(cfg, force, true); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}

	runCopy()

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				time.Sleep(100 * time.Millisecond)
				watcher.Add(configFile)
				runCopy()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}
