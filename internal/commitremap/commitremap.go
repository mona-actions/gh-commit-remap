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
	"sync/atomic"
)

const COMMIT_MAP_HEADER string = "old                                      new"

type File struct {
	FilePath string
	Prefix   string
}

// Parses the commit-map file and returns a map of old commit hashes to
// new commit hashes using the old commit sha as the key

func ParseCommitMap(filePath string) (*map[string]string, error) {
	commitMap := make(map[string]string)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(content)
	if buf.Len() == 0 {
		return &commitMap, nil
	}
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip adding the header to the map
		if line == COMMIT_MAP_HEADER {
			continue
		}
		fields := strings.Split(line, " ")
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid line: %s", line)
		}
		oldSha, newSha := fields[0], fields[1]
		commitMap[oldSha] = newSha
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &commitMap, nil
}

// Processes the files in the archive and updates the commit shas
func ProcessFiles(archiveLocation string, prefixes []string,
	commitMap *map[string]string, workers int) error {
	workerCount := workers
	fileChannel := make(chan File, workerCount)
	fileProcessWg := sync.WaitGroup{}
	filesToProcess := getAllFilesToProcess(prefixes, archiveLocation)
	totalFiles := len(filesToProcess)
	processedFiles := make(chan File, totalFiles)
	var processedFilesCount atomic.Int64

	// go routine to print out the progress of the processed files. It also
	// writes the processed files to a log file
	fmt.Printf("Processed %d/%d files\n", processedFilesCount, totalFiles)
	go func() {
		f, err := os.OpenFile("processed_files.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("error opening processed files log: %v", err)
		}
		defer f.Close()
		for file := range processedFiles {
			// Clear the previous line
			// \033 is the ASCII escape character
			// [1A moves the cursor up one line
			// [K erases the line
			// https://en.wikipedia.org/wiki/ANSI_escape_code
			fmt.Printf("\033[1A\033[K")
			fmt.Printf("Processed %d/%d files\n", processedFilesCount, totalFiles)
			if _, err := f.WriteString(fmt.Sprintf("%s\n", file.FilePath)); err != nil {
				log.Fatalf("error writing to processed files log: %v", err)
			}
		}
	}()
	// Starts a pool of workers to process the files
	for i := 0; i < workerCount; i++ {
		fileProcessWg.Add(1)
		go func() {
			defer fileProcessWg.Done()
			for file := range fileChannel {
				err := updateMetadataFile(file, *commitMap)
				if err != nil {
					log.Fatalf("error updating metadata file: %v", err)
				}
				processedFiles <- file
				processedFilesCount.Add(1)
			}
		}()
	}
	prefixWg := sync.WaitGroup{}
	// Seperate go routines to add the files to the channel
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

// Updates each metadata file with the new commit shas
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
	// Processes each of the different file types contained in the archive.
	// The file types listed below are currently the only types that contain
	// commit shas as a distinct field.
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
		return fmt.Errorf("no supported rewrite found for file type: %s", file.Prefix)
	}

	// Pretty print the data
	updatedData, err := json.MarshalIndent(dataMap, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling updated data: %v", err)
	}

	err = os.WriteFile(file.FilePath, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("error writing updated data: %v", err)
	}

	return nil
}

// Fetches all of the files to update based on the file prefixes
func getAllFilesToProcess(prefixes []string, archiveLocation string) []File {
	var files []File
	for _, prefix := range prefixes {
		filePaths, err := filepath.Glob(filepath.Join(archiveLocation, prefix+"_*.json"))
		for _, filePath := range filePaths {
			files = append(files, File{
				FilePath: filePath,
				Prefix:   prefix,
			})
		}
		if err != nil {
			log.Fatalf("error getting files: %v", err)
		}
	}
	return files
}
