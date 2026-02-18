# copytree

A CLI tool to copy files based on glob patterns, only when source is newer.

## Install

```bash
go install github.com/jeffwinkler/copytree@latest
```

## Usage

Create a `copytree.toml` in your working directory:

```toml
source = "/path/to/source"
destination = "/path/to/destination"
globs = ["**/*.txt", "**/*.go"]  # ** matches any directory depth
```

Run:

```bash
copytree           # uses copytree.toml in current directory
copytree -v        # verbose output
copytree -force    # overwrite even if target is newer
copytree -config path/to/config.toml
```

## Behavior

- Only copies if source file is newer than target
- Refuses to overwrite if target is newer (use `-force` to override)
- Creates intermediate directories as needed
- Preserves modification times
