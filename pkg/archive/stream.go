package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"

	pgzip "github.com/klauspost/pgzip"

	"github.com/mona-actions/gh-commit-remap/pkg/commitremap"
)

const maxMatchedFileSize = 2 << 30 // 2 GB guard for matched entries

// StreamRemap reads a .tar.gz migration archive, remaps SHAs in matching
// JSON metadata files in-flight, and writes the result to a new .tar.gz.
// This eliminates the extract→modify→retar cycle entirely.
//
// Files whose base name matches "<prefix>_<digits>.json" for any prefix in
// prefixes are read into memory, processed by replaceSHABytes, and written
// back. All other entries are streamed through unchanged.
//
// The output archive uses parallel gzip (pgzip) at BestSpeed for fast
// compression. Tar entry order from the input is preserved.
func StreamRemap(inArchive, outArchive string, commitMap map[string]string, prefixes []string) (commitremap.Stats, error) {
	stats := commitremap.Stats{PerFile: make(map[string]int)}

	if len(commitMap) == 0 {
		return stats, fmt.Errorf("commit map is empty; nothing to remap")
	}

	shaLen, err := commitremap.CommitMapSHALen(commitMap)
	if err != nil {
		return stats, fmt.Errorf("validating commit map: %w", err)
	}

	prefixSet := make(map[string]bool, len(prefixes))
	for _, p := range prefixes {
		prefixSet[p] = true
	}

	// Layout accumulators, filled during the single streaming pass below.
	// metadataDirs: distinct dirs holding matched SHA-bearing files.
	// unmatchedPrefixes: distinct prefixes of "<prefix>_<digits>.json" entries
	// that did not match prefixSet.
	dirSeen := make(map[string]bool)
	unmatchedSeen := make(map[string]bool)

	// Open input tar.gz
	inFile, err := os.Open(inArchive)
	if err != nil {
		return stats, fmt.Errorf("open input archive: %w", err)
	}
	defer inFile.Close()

	gzReader, err := gzip.NewReader(inFile)
	if err != nil {
		return stats, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	// Open output tar.gz
	outFile, err := os.Create(outArchive)
	if err != nil {
		return stats, fmt.Errorf("create output archive: %w", err)
	}

	gzWriter, _ := pgzip.NewWriterLevel(outFile, pgzip.BestSpeed)
	gzWriter.SetConcurrency(256<<10, runtime.NumCPU())
	tarWriter := tar.NewWriter(gzWriter)

	var tarClosed, gzClosed, fileClosed bool
	cleanup := func(retErr error) {
		if !tarClosed {
			_ = tarWriter.Close()
		}
		if !gzClosed {
			_ = gzWriter.Close()
		}
		if !fileClosed {
			_ = outFile.Close()
		}
		if retErr != nil {
			_ = os.Remove(outArchive)
		}
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup(err)
			return stats, fmt.Errorf("reading tar entry: %w", err)
		}

		// Safety validation (matching UnTar/ReTarDir behavior)
		if filepath.IsAbs(header.Name) {
			err := fmt.Errorf("archive entry %q has absolute path", header.Name)
			cleanup(err)
			return stats, err
		}
		if pathHasParentRef(header.Name) {
			err := fmt.Errorf("archive entry %q escapes destination", header.Name)
			cleanup(err)
			return stats, err
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg {
			err := fmt.Errorf("unsupported tar entry type %d for %q", header.Typeflag, header.Name)
			cleanup(err)
			return stats, err
		}

		// Clone the header to avoid aliasing
		hdr := *header

		if hdr.Typeflag == tar.TypeDir {
			if err := tarWriter.WriteHeader(&hdr); err != nil {
				cleanup(err)
				return stats, fmt.Errorf("writing dir header %q: %w", hdr.Name, err)
			}
			continue
		}

		// Classify this regular-file entry for layout reporting. Matched files
		// contribute their directory; unmatched "<prefix>_<digits>.json" files
		// contribute their prefix so callers can flag unrecognized metadata.
		if prefix, ok := commitremap.MetadataPrefix(hdr.Name); ok {
			if prefixSet[prefix] {
				dir := path.Dir(path.Clean(hdr.Name))
				if !dirSeen[dir] {
					dirSeen[dir] = true
					stats.MetadataDirs = append(stats.MetadataDirs, dir)
				}
			} else if !unmatchedSeen[prefix] {
				unmatchedSeen[prefix] = true
				stats.UnmatchedPrefixes = append(stats.UnmatchedPrefixes, prefix)
			}
		}

		// Check if this file matches a SHA-bearing prefix
		if hdr.Size >= 0 && commitremap.ShouldRemap(hdr.Name, prefixSet) {
			if hdr.Size > maxMatchedFileSize {
				err := fmt.Errorf("matched file %q is too large (%d bytes, max %d)", hdr.Name, hdr.Size, maxMatchedFileSize)
				cleanup(err)
				return stats, err
			}

			data, err := io.ReadAll(tarReader)
			if err != nil {
				cleanup(err)
				return stats, fmt.Errorf("reading matched file %q: %w", hdr.Name, err)
			}

			data, count := commitremap.ReplaceSHABytes(data, commitMap, shaLen)
			stats.FilesScanned++
			if count > 0 {
				stats.PerFile[hdr.Name] = count
			}

			// Size is unchanged (same-length SHA replacement), but set it
			// from actual data length for correctness.
			hdr.Size = int64(len(data))

			if err := tarWriter.WriteHeader(&hdr); err != nil {
				cleanup(err)
				return stats, fmt.Errorf("writing header for %q: %w", hdr.Name, err)
			}
			if _, err := tarWriter.Write(data); err != nil {
				cleanup(err)
				return stats, fmt.Errorf("writing data for %q: %w", hdr.Name, err)
			}
		} else {
			// Pass through unchanged
			if err := tarWriter.WriteHeader(&hdr); err != nil {
				cleanup(err)
				return stats, fmt.Errorf("writing passthrough header %q: %w", hdr.Name, err)
			}
			if hdr.Size > 0 {
				if _, err := io.Copy(tarWriter, tarReader); err != nil {
					cleanup(err)
					return stats, fmt.Errorf("copying passthrough file %q: %w", hdr.Name, err)
				}
			}
		}
	}

	// Close in order: tar → gzip → file
	if err := tarWriter.Close(); err != nil {
		cleanup(err)
		return stats, fmt.Errorf("closing tar writer: %w", err)
	}
	tarClosed = true
	if err := gzWriter.Close(); err != nil {
		cleanup(err)
		return stats, fmt.Errorf("closing gzip writer: %w", err)
	}
	gzClosed = true
	if err := outFile.Close(); err != nil {
		cleanup(err)
		return stats, fmt.Errorf("closing output file: %w", err)
	}
	fileClosed = true

	sort.Strings(stats.MetadataDirs)
	sort.Strings(stats.UnmatchedPrefixes)

	return stats, nil
}
