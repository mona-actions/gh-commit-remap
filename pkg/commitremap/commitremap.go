// Package commitremap rewrites SHAs inside GitHub migration archive
package commitremap

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// DefaultPrefixes returns the set of archive metadata file prefixes that
// gh-commit-remap rewrites by default. A fresh slice is returned on each
// call so callers can mutate the result without affecting other callers.
func DefaultPrefixes() []string {
	return []string{
		"issues",
		"issue_events",
		"issue_comments",
		"pull_requests",
		"pull_request_reviews",
		"pull_request_review_comments",
		"pull_request_review_threads",
		"commit_comments",
	}
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

	for i, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			lineErr := invalidCommitMapLineError{line: strings.TrimRight(line, "\r"), fields: len(fields)}
			return nil, fmt.Errorf("invalid commit map line: %w", lineErr)
		}

		// Skip the header line produced by git-filter-repo ("old new")
		if i == 0 && fields[0] == "old" && fields[1] == "new" {
			continue
		}

		commitMap[fields[0]] = fields[1]
	}

	return commitMap, nil
}

// ProcessFiles rewrites SHAs in JSON metadata files matching <prefix>_*.json inside archiveDir.
//
// Each file is scanned byte-by-byte using a sliding window that matches
// SHA-length hex sequences against the commit map. SHAs are replaced
// wherever they appear — including inside URLs, markdown, etc.
//
// numWorkers controls how many goroutines process files in parallel.
// If numWorkers <= 0, it defaults to runtime.NumCPU().
func ProcessFiles(archiveDir string, prefixes []string, commitMap map[string]string, numWorkers int) (Stats, error) {
	stats := Stats{PerFile: make(map[string]int)}

	shaLen, err := commitMapSHALen(commitMap)
	if err != nil {
		return stats, fmt.Errorf("validating commit map: %w", err)
	}

	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	// Collect all files to process
	var allFiles []string
	for _, prefix := range prefixes {
		pattern := filepath.Join(archiveDir, prefix+"_*.json")
		files, err := filepath.Glob(pattern)
		if err != nil {
			return stats, fmt.Errorf("globbing %s: %w", pattern, err)
		}
		allFiles = append(allFiles, files...)
	}

	stats.FilesScanned = len(allFiles)

	type fileResult struct {
		file  string
		count int
		err   error
	}

	results := make([]fileResult, len(allFiles))
	workCh := make(chan int, len(allFiles))
	for i := range allFiles {
		workCh <- i
	}
	close(workCh)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range workCh {
				n, err := updateMetadataFile(allFiles[idx], commitMap, shaLen)
				results[idx] = fileResult{file: allFiles[idx], count: n, err: err}
			}
		}()
	}
	wg.Wait()

	// Merge results in order, returning partial stats on first error
	var firstErr error
	for _, res := range results {
		if res.count > 0 {
			stats.PerFile[res.file] = res.count
		}
		if res.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("updating metadata file %s: %w", res.file, res.err)
		}
	}

	return stats, firstErr
}

func updateMetadataFile(filePath string, commitMap map[string]string, shaLen int) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("reading data: %w", err)
	}

	data, count := replaceSHABytes(data, commitMap, shaLen)
	if count == 0 {
		return 0, nil
	}

	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return count, fmt.Errorf("writing updated data: %w", err)
	}

	return count, nil
}

var hexTable [256]bool

// init initializes the hexTable with valid hexadecimal characters (valid sha1 and sha256 characters).
func init() {
	for _, b := range []byte("0123456789abcdefABCDEF") {
		hexTable[b] = true
	}
}

// isHexByte reports whether b is a valid hexadecimal byte (0-9, a-f, A-F).
// Uses a precomputed lookup table for branchless evaluation.
func isHexByte(b byte) bool {
	return hexTable[b]
}

// commitMapSHALen returns the SHA length common to every key in commitMap.
// It returns an error if the map is empty or if keys/values have different lengths.
func commitMapSHALen(commitMap map[string]string) (int, error) {
	shaLen := 0
	for old, new_ := range commitMap {
		if shaLen == 0 {
			shaLen = len(old)
			if shaLen == 0 {
				return 0, fmt.Errorf("commit map contains an empty key")
			}
		}
		if len(old) != shaLen || len(new_) != shaLen {
			return 0, fmt.Errorf("commit map SHAs have inconsistent lengths: expected %d, got key len %d / value len %d", shaLen, len(old), len(new_))
		}
	}
	if shaLen == 0 {
		return 0, fmt.Errorf("commit map is empty")
	}
	return shaLen, nil
}

// replaceSHABytes scans data byte-by-byte using a sliding window of shaLen.
//
// Algorithm:
//  1. Walk each byte, counting consecutive valid hex (SHA) bytes.
//  2. When a non-hex byte is hit, reset the counter, no SHA can span it.
//  3. Once we have shaLen consecutive hex bytes, extract that window and
//     look it up in commitMap.
//  4. On match: replace in-place, skip past the replaced bytes. The next
//     window starts fresh from the byte after the replacement, avoiding
//     re-scanning the bytes we just wrote.
//  5. On no match: keep going. The counter grows past shaLen so the
//     window slides forward by one byte each step, checking every
//     overlapping shaLen-sized substring. For example with shaLen=40,
//     if bytes 0–39 don't match, bytes 1–40 are checked next, etc.
//
// Returns the (potentially modified) byte slice and the replacement count.
func replaceSHABytes(data []byte, commitMap map[string]string, shaLen int) ([]byte, int) {
	count := 0
	consecutiveHex := 0

	for i := 0; i < len(data); i++ {
		if isHexByte(data[i]) {
			consecutiveHex++
		} else {
			// Non-hex byte breaks any potential SHA sequence.
			consecutiveHex = 0
			continue
		}

		// Once we have enough consecutive hex bytes, check if the last
		// shaLen bytes match an entry in the commit map.
		if consecutiveHex >= shaLen {
			start := i - shaLen + 1
			candidate := string(data[start : i+1])
			if newSHA, ok := commitMap[candidate]; ok {
				copy(data[start:i+1], newSHA)
				count++
				consecutiveHex = 0
			}
			// If no match, consecutiveHex keeps growing and the window
			// slides forward on the next iteration.
		}
	}

	return data, count
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
