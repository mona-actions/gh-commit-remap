// Package commitremap rewrites SHAs inside GitHub migration archive
package commitremap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPrefixes returns the set of archive metadata file prefixes that
// gh-commit-remap rewrites by default. A fresh slice is returned on each
// call so callers can mutate the result without affecting other callers.
func DefaultPrefixes() []string {
	return []string{"pull_requests", "issues", "issue_events"}
}

type invalidCommitMapLineError struct {
	line   string
	fields int
}
type Stats struct {
	FilesScanned int
	PerFile      map[string]int
}

func (e invalidCommitMapLineError) Error() string {
	return fmt.Sprintf("line %q has %d fields", e.line, e.fields)
}

// ParseCommitMap parses a commit-map file into a map of old to new SHAs.
// The file must contain one "old new" pair per line, matching git filter-repo format.
func ParseCommitMap(filePath string) (map[string]string, error) {
	commitMap := make(map[string]string)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading commit map %s: %w", filePath, err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			lineErr := invalidCommitMapLineError{line: strings.TrimRight(line, "\r"), fields: len(fields)}
			return nil, fmt.Errorf("invalid commit map line: %w", lineErr)
		}

		commitMap[fields[0]] = fields[1]
	}

	return commitMap, nil
}

// ProcessFiles rewrites SHAs in JSON metadata files matching <prefix>_*.json inside archiveDir.
//
// Each file is walked once, replacing string values that exactly match a key in
// commitMap. Only whole-string SHA values are replaced. SHAs embedded in URLs,
// markdown, or composite strings are not rewritten.
func ProcessFiles(archiveDir string, prefixes []string, commitMap map[string]string) (Stats, error) {
	stats := Stats{PerFile: make(map[string]int)}

	for _, prefix := range prefixes {
		pattern := filepath.Join(archiveDir, prefix+"_*.json")
		files, err := filepath.Glob(pattern)
		if err != nil {
			return stats, fmt.Errorf("globbing %s: %w", pattern, err)
		}

		for _, file := range files {
			stats.FilesScanned++
			n, err := updateMetadataFile(file, commitMap)
			if err != nil {
				return stats, fmt.Errorf("updating metadata file %s: %w", file, err)
			}
			if n > 0 {
				stats.PerFile[file] = n
			}
		}
	}

	return stats, nil
}

func updateMetadataFile(filePath string, commitMap map[string]string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("reading data: %w", err)
	}

	var dataMap interface{}
	err = json.Unmarshal(data, &dataMap)
	if err != nil {
		return 0, fmt.Errorf("unmarshaling data: %w", err)
	}

	count := replaceSHA(dataMap, commitMap)
	if count == 0 {
		return 0, nil
	}

	updatedData, err := json.MarshalIndent(dataMap, "", "  ")
	if err != nil {
		return count, fmt.Errorf("marshaling updated data: %w", err)
	}

	err = os.WriteFile(filePath, updatedData, 0644)
	if err != nil {
		return count, fmt.Errorf("writing updated data: %w", err)
	}

	return count, nil
}

// replaceSHA walks data in place, rewriting whole-string values that match a
// key in commitMap. It returns the number of replacements performed.
func replaceSHA(data interface{}, commitMap map[string]string) int {
	count := 0
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if str, ok := value.(string); ok {
				if newSHA, hit := commitMap[str]; hit {
					v[key] = newSHA
					count++
				}
				continue
			}

			count += replaceSHA(value, commitMap)
		}
	case []interface{}:
		for i, value := range v {
			if str, ok := value.(string); ok {
				if newSHA, hit := commitMap[str]; hit {
					v[i] = newSHA
					count++
				}
				continue
			}

			count += replaceSHA(value, commitMap)
		}
	}
	return count
}

// summarize the work performed by a ProcessFiles call.
//
// FilesScanned counts every metadata file inspected

// FilesChanged returns the number of files in which at least one SHA was rewritten.
func (s Stats) FilesChanged() int { return len(s.PerFile) }

// TotalReplacements returns the total number of SHA replacements across all files.
func (s Stats) TotalReplacements() int {
	total := 0
	for _, n := range s.PerFile {
		total += n
	}
	return total
}
