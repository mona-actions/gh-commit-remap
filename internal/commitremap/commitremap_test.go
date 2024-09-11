package commitremap

import (
	"os"
	"reflect"
	"testing"
)

func TestParseCommitMap(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expected    *map[string]string
		expectError bool
	}{
		{
			name: "Valid commit map",
			fileContent: `oldSHA1 newSHA1
oldSHA2 newSHA2
oldSHA3 newSHA3`,
			expected: &map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
				"oldSHA3": "newSHA3",
			},
			expectError: false,
		},
		{
			name:        "Empty file",
			fileContent: ``,
			expected:    &map[string]string{},
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
		{
			name: "Skips first line (old .... new) and reads the rest",
			fileContent: `old                                      new
oldSHA1 newSHA1
oldSHA2 newSHA2`,
			expected: &map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
			},
			expectError: false,
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

			// Compare maps for equality and length
			if !reflect.DeepEqual(*result, *tt.expected) {
				t.Errorf("Expected %+v, got %+v", *tt.expected, *result)
			}
		})
	}
}
