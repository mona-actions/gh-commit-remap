package archive

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// reTarFiles creates a new tar archive from the files in the given directory.
// The name of the archive is the same as the directory name.
func ReTar(archivePath string) (string, error) {
	// Extract the directory name from the archivePath
	dirName := filepath.Base(archivePath)

	// Create the name of the new archive with -REMAPPED suffix
	archiveName := fmt.Sprintf("%s-REMAPPED.tar.gz", dirName)

	err := checkTarAvailability()
	if err != nil {
		return "", err
	}

	// Create and run the tar command
	tarCmd := exec.Command("tar", "-czf", archiveName, "-C", archivePath, ".")
	err = tarCmd.Run()
	if err != nil {
		return "", fmt.Errorf("error re-tarring the files: %w", err)
	}

	return archiveName, nil
}

// UnTar extracts the given .tar.gz archive into the specified destination directory.
// If destDir is empty, a directory will be created in the current working directory
// using the archive base name (without the .tar.gz suffix). It returns the directory
// where the archive was extracted.
func UnTar(archiveFile, destDir string) (string, error) {
	if archiveFile == "" {
		return "", fmt.Errorf("archive file path is empty")
	}

	if err := checkTarAvailability(); err != nil {
		return "", err
	}

	// Ensure the archive exists before attempting extraction
	if _, err := os.Stat(archiveFile); err != nil {
		return "", fmt.Errorf("cannot stat archive file: %w", err)
	}

	// Determine destination directory if not provided
	if destDir == "" {
		base := filepath.Base(archiveFile)
		// strip .tar.gz if present
		base = strings.TrimSuffix(base, ".tar.gz")
		if base == filepath.Base(archiveFile) { // no .tar.gz suffix
			base = strings.TrimSuffix(base, ".tgz")
		}
		if base == "" {
			base = "extracted"
		}
		destDir = base
	}

	// Create destination directory if it does not exist
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Run tar extraction
	tarCmd := exec.Command("tar", "-xzf", archiveFile, "-C", destDir)
	if err := tarCmd.Run(); err != nil {
		return "", fmt.Errorf("error extracting archive: %w", err)
	}

	return destDir, nil
}

// checkTarAvailability checks if the 'tar' command is available in the system's PATH.
func checkTarAvailability() error {
	_, err := exec.LookPath("tar")
	return err
}
