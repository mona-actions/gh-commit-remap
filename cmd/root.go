/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	// Adjust the file path as necessary
	mapPath := "test/TestRepo.git/filter-repo/commit-map"
	commitMap, err := parseCommitMap(mapPath)
	if err != nil {
		log.Fatalf("Error parsing commit map: %v", err)
	}

	// Adjust the file path as necessary
	prPath := "test/3723ff5e-4b7e-11ef-9bf5-2aca377420b3/pull_requests_000001.json"

	// Read the JSON file containing the pull request metadata
	prData, err := os.ReadFile(prPath)
	if err != nil {
		log.Fatalf("Error reading pull request data: %v", err)
	}

	var prDataMap interface{}
	err = json.Unmarshal(prData, &prDataMap)
	if err != nil {
		log.Fatalf("Error unmarshaling pull request data: %v", err)
	}

	// Iterate over the commit map and replace the old commit hashes with the new commit hashes
	for _, commit := range *commitMap {
		replaceSHA(prDataMap, commit.Old, commit.New)
	}

	// Marshal the updated pull request metadata to JSON and pretty print it

	updatedPrData, err := json.MarshalIndent(prDataMap, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling updated pull request data: %v", err)
	}

	// Overwrite the original file with the updated data
	err = os.WriteFile(prPath, updatedPrData, 0644)
	if err != nil {
		log.Fatalf("Error writing updated pull request data: %v", err)
	}
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

func replaceSHA(data interface{}, oldSHA, newSHA string) {
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
