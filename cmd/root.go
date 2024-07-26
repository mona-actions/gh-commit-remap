/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
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
	filePath := "test/commit-map"
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

func startOrgMigration(org string, repos []string) error {
	// Implement the org migration here
	return nil
}

func downloadOrgMigrationArchive(org string) error {
	// Implement the download of the org migration archive here
	return nil
}

func applyCommitMap(commitMap *[]CommitMapEntry) error {
	// Implement the application of the commit map here
	return nil
}
