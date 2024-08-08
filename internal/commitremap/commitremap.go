package commitremap

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Struct to represent a single entry in the commit map
type CommitMapEntry struct {
	Old string
	New string
}

// Parses the file and returns a map of old commit hashes to new commit hashes
func ParseCommitMap(filePath string) (*[]CommitMapEntry, error) {
	commitMap := []CommitMapEntry{}

	// Read the commit-map file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	// Split the file content into lines
	lines := strings.Split(string(content), "\n")

	// Iterate over the lines and parse the old and new commit hashes
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid line: %s", line)
		}

		commitMap = append(commitMap, CommitMapEntry{
			Old: fields[0],
			New: fields[1],
		})
	}
	return &commitMap, nil
}

func ProcessFiles(archiveLocation string, prefixes []string, commitMap *[]CommitMapEntry) error {

	for _, prefix := range prefixes {
		// Get a list of all files that match the pattern
		files, err := filepath.Glob(filepath.Join(archiveLocation, prefix+"_*.json"))
		if err != nil {
			log.Fatalf("Error getting files: %v", err)
		}

		// Process each file
		for _, file := range files {
			log.Println("Processing file:", file)

			err := updateMetadataFile(file, commitMap)
			if err != nil {
				return fmt.Errorf("Error updating metadata file: %v; %v", file, err)
			}
		}
	}
	return nil
}

func updateMetadataFile(filePath string, commitMap *[]CommitMapEntry) error {
	// Read the JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("Error reading data: %v", err)
	}

	var dataMap interface{}
	err = json.Unmarshal(data, &dataMap)
	if err != nil {
		return fmt.Errorf("Error unmarshaling data: %v", err)
	}

	// Iterate over the commit map and replace the old commit hashes with the new ones
	for _, commit := range *commitMap {
		replaceSHA(dataMap, commit.Old, commit.New)
	}

	// Marshal the updated data to JSON and pretty print it
	updatedData, err := json.MarshalIndent(dataMap, "", "  ")
	if err != nil {
		return fmt.Errorf("Error marshaling updated data: %v", err)
	}

	// Overwrite the original file with the updated data
	err = os.WriteFile(filePath, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("Error writing updated data: %v", err)
	}

	return nil
}

func replaceSHA(data interface{}, oldSHA string, newSHA string) {
	if data == nil {
		return
	}

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if str, ok := value.(string); ok && str == oldSHA {
				v[key] = newSHA
			} else {
				replaceSHA(value, oldSHA, newSHA)
			}
		}
	case []interface{}:
		for i, value := range v {
			if str, ok := value.(string); ok && str == oldSHA {
				v[i] = newSHA
			} else {
				replaceSHA(value, oldSHA, newSHA)
			}
		}
	default:
		// Unsupported type, do nothing
	}
}
