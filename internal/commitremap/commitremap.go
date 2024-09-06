package commitremap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type File struct {
	FilePath string
	Prefix   string
}

// Parses the file and returns a map of old commit hashes to new commit hashes
func ParseCommitMap(filePath string) (*map[string]string, error) {
	commitMap := make(map[string]string)
	// Read the commit-map file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(content)
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip adding the header to the map
		if line == "old                                      new" {
			continue
		}
		fields := strings.Split(line, " ")
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid line: %s", line)
		}
		commitMap[fields[0]] = fields[1]
	}
	return &commitMap, nil
}

func ProcessFiles(archiveLocation string, prefixes []string,
	commitMap *map[string]string, workers int) error {
	workerCount := 10
	fileChannel := make(chan File, workerCount)
	fileProcessWg := sync.WaitGroup{}
	filesToProcess := getAllFilesToProcess(prefixes, archiveLocation)
	totalFiles := len(filesToProcess)
	processedFiles := make(chan File, totalFiles)
	processedFilesCount := 0
	// go routine to print out the progress of the processed files. It also
	// writes the processed files to a log file
	fmt.Printf("Processed %d/%d files\n", processedFilesCount, totalFiles)
	go func() {
		f, err := os.OpenFile("processed_files.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Error opening processed files log: %v", err)
		}
		defer f.Close()
		for file := range processedFiles {
			fmt.Printf("\033[1A\033[K")
			fmt.Printf("Processed %d/%d files\n", processedFilesCount, totalFiles)
			if _, err := f.WriteString(fmt.Sprintf("%s\n", file.FilePath)); err != nil {
				log.Fatalf("Error writing to processed files log: %v", err)
			}
		}
	}()

	for i := 0; i < workerCount; i++ {
		fileProcessWg.Add(1)
		go func() {
			defer fileProcessWg.Done()
			for file := range fileChannel {
				err := updateMetadataFile(file, *commitMap)
				if err != nil {
					log.Fatalf("Error updating metadata file: %v", err)
				}
				processedFiles <- file
				processedFilesCount++
			}
		}()
	}
	prefixWg := sync.WaitGroup{}
	// Add the files to the channel
	for _, file := range filesToProcess {
		prefixWg.Add(1)
		go func(file File) {
			defer prefixWg.Done()
			fileChannel <- file
		}(file)
	}
	prefixWg.Wait()
	close(fileChannel)
	fileProcessWg.Wait()
	close(processedFiles)
	return nil
}

func updateMetadataFile(file File, commitMap map[string]string) error {
	var dataMap []interface{}
	data, err := os.ReadFile(file.FilePath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &dataMap)
	if err != nil {
		return err
	}
	switch {
	case file.Prefix == "pull_requests":
		updatePullRequests(commitMap, &dataMap)
	case file.Prefix == "pull_request_review_comments":
		updatePullRequestReviewComments(commitMap, &dataMap)
	case file.Prefix == "pull_request_reviews":
		updatePullRequestReviews(commitMap, &dataMap)
	case file.Prefix == "pull_request_review_threads":
		updatePullRequestReviewThreads(commitMap, &dataMap)
	case file.Prefix == "commit_comments":
		updateCommitComments(commitMap, &dataMap)
	default:
		return fmt.Errorf("No supported rewrite found for file type: %s", file.Prefix)
	}

	// Pretty print the data
	updatedData, err := json.MarshalIndent(dataMap, "", "  ")
	if err != nil {
		return fmt.Errorf("Error marshaling updated data: %v", err)
	}

	err = os.WriteFile(file.FilePath, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("Error writing updated data: %v", err)
	}

	return nil
}

func getAllFilesToProcess(prefixes []string, archiveLocation string) []File {
	var files []File
	for _, prefix := range prefixes {
		// Get a list of all filePaths that match the pattern
		filePaths, err := filepath.Glob(filepath.Join(archiveLocation, prefix+"_*.json"))
		for _, filePath := range filePaths {
			files = append(files, File{
				FilePath: filePath,
				Prefix:   prefix,
			})
		}
		if err != nil {
			log.Fatalf("Error getting files: %v", err)
		}
	}
	return files
}
