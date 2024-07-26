package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// Struct to represent a single entry in the commit map
type CommitMapEntry struct {
	Old string `yaml:"old"`
	New string `yaml:"new"`
}

func main() {
	// Adjust the file path as necessary
	filePath := "commit-map"
	commitMap, err := parseCommitMap(filePath)
	if err != nil {
		log.Fatalf("Error parsing commit map: %v", err)
	}

	fmt.Println(commitMap)
}

// Parses the file and returns a map of old commit hashes to new commit hashes
func parseCommitMap(filePath string) (*[]CommitMapEntry, error) {

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
