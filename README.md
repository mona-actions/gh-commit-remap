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
  -m, --migration-archive string   Path to the migration archive Example: /path/to/migration-archive.tar.gz
```

The mapping file should be in the format produced by [git-filter-repo](https://github.com/newren/git-filter-repo). For example:
```
old                                      new
892d146b368d74a64ad8a83af8ecc949ff20a408 0000000000000000000000000000000000000000
12f9bdd865d5f8b19eaaff8c94af10ec1d7f6e27 26b42504e90f3cb96e684be18ffbc3ff42d12383
...
```

This tool will produce a new migration archive in the current directory, appending `-REMAPPED` to the base filename of the original migration archive. 