/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Struct to represent a single entry in the commit map
type CommitMapEntry struct {
	Old string
	New string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-commit-remap",
	Short: "remaps commit hashes in a GitHub archive",
	Long: `Is a CLI tool that can remap commits hashed 
	after performing a history re-write when performing a migration For exam`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: main,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gh-commit-remap.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().StringP("mapping-file", "c", "", "Path to the commit map file Example: /path/to/commit-map")
	rootCmd.MarkFlagRequired("mapping-file")

	rootCmd.Flags().StringP("migration-archive", "m", "", "Path to the migration archive Example: /path/to/migration-archive.tar.gz")
	rootCmd.MarkFlagRequired("migration-archive")
}

func main(cmd *cobra.Command, args []string) {
	// leaving this for now to quickly test the code
	//mapPath := "test/TestRepo.git/filter-repo/commit-map"
	mapPath, _ := cmd.Flags().GetString("mapping-file")
	commitMap, err := parseCommitMap(mapPath)
	if err != nil {
		log.Fatalf("Error parsing commit map: %v", err)
	}

	// config to define the types of files to process
	types := []string{"pull_requests", "issues"}

	// leaving this for now to quickly test the code
	//archivePath := "test/3723ff5e-4b7e-11ef-9bf5-2aca377420b3"
	archivePath, _ := cmd.Flags().GetString("migration-archive")

	processFiles(archivePath, types, commitMap)
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

func updateMetadataFile(filePath string, commitMap *[]CommitMapEntry) {
	// Read the JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading data: %v", err)
	}

	var dataMap interface{}
	err = json.Unmarshal(data, &dataMap)
	if err != nil {
		log.Fatalf("Error unmarshaling data: %v", err)
	}

	// Iterate over the commit map and replace the old commit hashes with the new ones
	for _, commit := range *commitMap {
		replaceSHA(dataMap, commit.Old, commit.New)
	}

	// Marshal the updated data to JSON and pretty print it
	updatedData, err := json.MarshalIndent(dataMap, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling updated data: %v", err)
	}

	// Overwrite the original file with the updated data
	err = os.WriteFile(filePath, updatedData, 0644)
	if err != nil {
		log.Fatalf("Error writing updated data: %v", err)
	}
}

func processFiles(archiveLocation string, prefixes []string, commitMap *[]CommitMapEntry) {

	for _, prefix := range prefixes {
		// Get a list of all files that match the pattern
		files, err := filepath.Glob(filepath.Join(archiveLocation, prefix+"_*.json"))
		if err != nil {
			log.Fatalf("Error getting files: %v", err)
		}

		// Process each file
		for _, file := range files {
			log.Println("Processing file:", file)

			updateMetadataFile(file, commitMap)
		}
	}
}
