package commitremap

import (
	"os"
	"testing"
)

func TestParseCommitMap(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expected    *[]CommitMapEntry
		expectError bool
	}{
		{
			name: "Valid commit map",
			fileContent: `oldSHA1 newSHA1
oldSHA2 newSHA2
oldSHA3 newSHA3`,
			expected: &[]CommitMapEntry{
				{Old: "oldSHA1", New: "newSHA1"},
				{Old: "oldSHA2", New: "newSHA2"},
				{Old: "oldSHA3", New: "newSHA3"},
			},
			expectError: false,
		},
		{
			name:        "Empty file",
			fileContent: ``,
			expected:    &[]CommitMapEntry{},
			expectError: false,
		},
		{
			name: "Invalid line format",
			fileContent: `oldSHA1 newSHA1
invalidLine
oldSHA2 newSHA2`,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file
			tmpfile, err := os.CreateTemp("", "commitmap")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpfile.Name())

			// Write the test content to the temporary file
			if _, err := tmpfile.WriteString(tt.fileContent); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			// Call the function under test
			result, err := ParseCommitMap(tmpfile.Name())

			// Check for expected error
			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				return
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}

			// Check the result
			if len(*result) != len(*tt.expected) {
				t.Fatalf("Expected %d entries, got %d", len(*tt.expected), len(*result))
			}
			for i, entry := range *result {
				if entry.Old != (*tt.expected)[i].Old || entry.New != (*tt.expected)[i].New {
					t.Errorf("Expected entry %d to be %+v, got %+v", i, (*tt.expected)[i], entry)
				}
			}
		})
	}
}
