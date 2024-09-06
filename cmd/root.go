/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"log"
	"os"

	"github.com/mona-actions/gh-commit-remap/internal/archive"
	"github.com/mona-actions/gh-commit-remap/internal/commitremap"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.Flags().StringP("mapping-file", "c", "", "Path to the commit map file Example: /path/to/commit-map")
	rootCmd.MarkFlagRequired("mapping-file")

	rootCmd.Flags().StringP("migration-archive", "m", "", "Path to the migration archive Example: /path/to/migration-archive.tar.gz")
	rootCmd.MarkFlagRequired("migration-archive")

	// Optional flag to specify the number of threads to use for processing
	rootCmd.Flags().StringP("number-of-threads", "t", "10", "[OPTIONAL] Number of threads(goroutines) to use for processing. Defaults to 10")
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-commit-remap",
	Short: "remaps commit hashes in a GitHub archive",
	Long: `Is a CLI tool that can remap commits hashed 
	after performing a history re-write when performing a migration For exam`,
	Run: func(cmd *cobra.Command, args []string) {
		mapPath, _ := cmd.Flags().GetString("mapping-file")
		commitMap, err := commitremap.ParseCommitMap(mapPath)
		if err != nil {
			log.Fatalf("Error parsing commit map: %v", err)
		}

		// config to define the types of files to process
		types := []string{"pull_requests", "pull_request_reviews", "pull_request_review_comments", "pull_request_review_threads", "commit_comments"}

		archivePath, _ := cmd.Flags().GetString("migration-archive")
		workers, _ := cmd.Flags().GetInt("number-of-threads")
		err = commitremap.ProcessFiles(archivePath, types, commitMap, workers)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Processed files successfully, re-taring archive")
		tarPath, err := archive.ReTar(archivePath)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("New archive created: %s", tarPath)

	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
