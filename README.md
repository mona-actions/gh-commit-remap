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
  -m, --migration-archive string   Path to the migration archive Example: /path/to/migration-archive or /path/to/migration-archive.tar.gz
```

The mapping file should be in the format produced by [git-filter-repo](https://github.com/newren/git-filter-repo). For example:
```
old                                      new
892d146b368d74a64ad8a83af8ecc949ff20a408 0000000000000000000000000000000000000000
12f9bdd865d5f8b19eaaff8c94af10ec1d7f6e27 26b42504e90f3cb96e684be18ffbc3ff42d12383
...
```

You can provide a directory where your migration archive has been expanded, or a `.tar.gz` file, via the `--migration-archive` argument. In either case, this tool will produce a new migration archive in the current directory, appending `-REMAPPED` to the base filename of the original migration archive.

## Using as a library

The remap and archive packaging logic is also available as a Go library under `pkg/`:

```go
package main

import (
	"log"

	"github.com/mona-actions/gh-commit-remap/pkg/archive"
	"github.com/mona-actions/gh-commit-remap/pkg/commitremap"
)

func main() {
	// 1. Extract a .tar.gz migration archive (auto-creates destDir).
	extracted, err := archive.UnTar("migration-archive.tar.gz", "")
	if err != nil {
		log.Fatal(err)
	}

	// 2. Parse the commit-map produced by `git filter-repo`.
	commitMap, err := commitremap.ParseCommitMap("commit-map")
	if err != nil {
		log.Fatal(err)
	}

	// 3. Rewrite SHAs in the archive's metadata JSON files.
	if err := commitremap.ProcessFiles(extracted, commitremap.DefaultPrefixes(), commitMap); err != nil {
		log.Fatal(err)
	}

	// 4. Re-package to a path of your choosing.
	if err := archive.ReTarDir(extracted, "migration-archive-REMAPPED.tar.gz"); err != nil {
		log.Fatal(err)
	}
}
```

### Known limitations

- **Whole-string SHA match only.** `ProcessFiles` only rewrites JSON string
  values that exactly equal a commit-map key. SHAs embedded in URLs
  (`/commits/<sha>`), markdown bodies, or `_links.*.href` are not rewritten.
  Tracked as a follow-up issue.
- **Limited prefix coverage.** `DefaultPrefixes` covers `pull_requests`,
  `issues`, `issue_events`. Other archive files containing SHAs
  (`commit_comments_*.json`, `pull_request_review*_*.json`,
  `releases_*.json`'s `target_commitish`, etc.) are not processed by default.
  Pass a custom `prefixes` slice to `ProcessFiles` to widen coverage, or
  track the follow-up issue for a default-list expansion.