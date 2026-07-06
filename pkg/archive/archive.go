package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	pgzip "github.com/klauspost/pgzip"
)

// UnTar decompresses a .tar.gz file into destDir, returning the directory containing the extracted contents.
func UnTar(archiveFile, destDir string) (string, error) {
	if archiveFile == "" {
		return "", fmt.Errorf("archive file path is empty")
	}

	if _, err := os.Stat(archiveFile); err != nil {
		return "", fmt.Errorf("cannot stat archive file: %w", err)
	}

	file, err := os.Open(archiveFile)
	if err != nil {
		return "", fmt.Errorf("cannot open archive file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer gzipReader.Close()

	if destDir == "" {
		base := filepath.Base(archiveFile)
		base = strings.TrimSuffix(base, ".tar.gz")
		if base == filepath.Base(archiveFile) {
			base = strings.TrimSuffix(base, ".tgz")
		}
		if base == "" {
			base = "extracted"
		}
		destDir = base
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return "", fmt.Errorf("failed to open destination root: %w", err)
	}
	defer root.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error reading tar entry: %w", err)
		}

		if filepath.IsAbs(header.Name) {
			return "", fmt.Errorf("archive entry %q has absolute path", header.Name)
		}
		if pathHasParentRef(header.Name) {
			return "", fmt.Errorf("archive entry %q escapes destination", header.Name)
		}
		relPath := filepath.ToSlash(filepath.Clean(header.Name))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(relPath, header.FileInfo().Mode().Perm()); err != nil {
				return "", fmt.Errorf("failed to create directory %q: %w", relPath, err)
			}
		case tar.TypeReg:
			if header.Size < 0 {
				return "", fmt.Errorf("archive entry %q has negative size", header.Name)
			}
			parent := filepath.ToSlash(filepath.Dir(relPath))
			if err := root.MkdirAll(parent, 0o755); err != nil {
				return "", fmt.Errorf("failed to create parent directory for %q: %w", relPath, err)
			}
			outFile, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode().Perm())
			if err != nil {
				return "", fmt.Errorf("failed to create file %q: %w", relPath, err)
			}
			_, copyErr := io.CopyN(outFile, tarReader, header.Size)
			closeErr := outFile.Close()
			if copyErr != nil {
				return "", fmt.Errorf("failed to extract file %q: %w", relPath, copyErr)
			}
			if closeErr != nil {
				return "", fmt.Errorf("failed to close file %q: %w", relPath, closeErr)
			}
		default:
			return "", fmt.Errorf("unsupported tar entry type %d for %q", header.Typeflag, header.Name)
		}
	}

	return destDir, nil
}

func pathHasParentRef(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// ReTarDir creates a .tar.gz archive at outPath from the contents of srcDir.
func ReTarDir(srcDir, outPath string) (retErr error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("cannot stat source directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path %q is not a directory", srcDir)
	}

	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf("failed to resolve source directory: %w", err)
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("failed to resolve archive path: %w", err)
	}
	absSrc = filepath.Clean(absSrc)
	absOut = filepath.Clean(absOut)
	if absOut == absSrc || strings.HasPrefix(absOut, absSrc+string(os.PathSeparator)) {
		return fmt.Errorf("output path %q must not be inside source directory %q", outPath, srcDir)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("failed to create archive parent directory: %w", err)
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	gzipWriter, _ := pgzip.NewWriterLevel(outFile, pgzip.BestSpeed)
	gzipWriter.SetConcurrency(256<<10, runtime.NumCPU())
	tarWriter := tar.NewWriter(gzipWriter)

	// The success path closes each writer explicitly (in tar -> gzip -> file order)
	var tarClosed, gzipClosed, fileClosed bool
	defer func() {
		if !tarClosed {
			_ = tarWriter.Close()
		}
		if !gzipClosed {
			_ = gzipWriter.Close()
		}
		if !fileClosed {
			_ = outFile.Close()
		}
		if retErr != nil {
			_ = os.Remove(outPath)
		}
	}()

	rootHeader, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("failed to create root tar header: %w", err)
	}
	rootHeader.Name = "./"
	if err := tarWriter.WriteHeader(rootHeader); err != nil {
		return fmt.Errorf("failed to write root tar header: %w", err)
	}

	walkErr := filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		// Skip the output archive file if it somehow appears in the walk (defensive; guarded above).

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to read file info for %q: %w", path, err)
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to calculate archive path for %q: %w", path, err)
		}
		tarPath := filepath.ToSlash(relPath)

		// Reject special files (symlinks, devices, FIFOs, sockets)
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type for %q: only regular files and directories are archivable", path)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %q: %w", path, err)
		}
		header.Name = "./" + tarPath
		if info.IsDir() {
			header.Name += "/"
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %q: %w", path, err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		inFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open source file %q: %w", path, err)
		}
		_, copyErr := io.Copy(tarWriter, inFile)
		closeErr := inFile.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to write file %q to archive: %w", path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close source file %q: %w", path, closeErr)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("failed to walk source directory: %w", walkErr)
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	tarClosed = true
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}
	gzipClosed = true
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("failed to close archive: %w", err)
	}
	fileClosed = true

	return nil
}
