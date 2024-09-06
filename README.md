# gh-commit-remap

CLI tool to remap commits during a migration after a history rewrite

## Install

```bash
gh extension install mona-actions/gh-commit-remap
```

## Upgrade

```bash
gh extension upgrade gh-commit-remap
```

## Usage

```bash
gh commit-remap --help
Is a CLI tool that can remap commits hashed 
        after performing a history re-write when performing a migration For exam

Usage:
  gh-commit-remap [flags]

Flags:
  -h, --help                       help for gh-commit-remap
  -c, --mapping-file string        Path to the commit map file Example: /path/to/commit-map
  -m, --migration-archive string   Path to the migration archive Example: /path/to/migration-archive
  -t, --number-of-threads int      [OPTIONAL] Number of threads(goroutines) to use for processing. Defaults to 10"
```