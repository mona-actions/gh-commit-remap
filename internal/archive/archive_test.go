package archive

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReTar(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file in the temporary directory
	tempFile := filepath.Join(tempDir, "testfile")
	origContent := []byte("test")
	if err := os.WriteFile(tempFile, origContent, 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	// Call the function under test
	tarFile, err := ReTar(tempDir)
	if err != nil {
		t.Fatalf("ReTar failed: %v", err)
	}

	// Check if the tar file was created
	if _, err := os.Stat(tarFile); os.IsNotExist(err) {
		t.Fatalf("Tar file was not created: %v", err)
	}

	// Ensure the tar file is removed after the test
	defer os.Remove(tarFile)

	// Extract the tar file to a new directory
	extractDir, err := os.MkdirTemp("", "extract")
	if err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}
	defer os.RemoveAll(extractDir)

	tarCmd := exec.Command("tar", "-xzf", tarFile, "-C", extractDir)
	err = tarCmd.Run()
	if err != nil {
		t.Fatalf("Failed to extract tar file: %v", err)
	}

	// Read the contents of the extracted file
	extractedFile := filepath.Join(extractDir, "testfile")
	extractedContent, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	// Compare the contents of the original file and the extracted file
	if !bytes.Equal(origContent, extractedContent) {
		t.Fatalf("Original file content and extracted file content do not match")
	}
}
