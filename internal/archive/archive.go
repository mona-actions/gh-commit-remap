package archive

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// reTarFiles creates a new tar archive from the files in the given directory.
// The name of the archive is the same as the directory name.
func ReTar(archivePath string) (string, error) {
	// Extract the directory name from the archivePath
	dirName := filepath.Base(archivePath)

	// Create the name of the new archive
	archiveName := dirName + ".tar.gz"

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

// checkTarAvailability checks if the 'tar' command is available in the system's PATH.
func checkTarAvailability() error {
	_, err := exec.LookPath("tar")
	return err
}
