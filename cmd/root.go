/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"log"
	"os"
	"strings"

	"github.com/mona-actions/gh-commit-remap/pkg/archive"
	"github.com/mona-actions/gh-commit-remap/pkg/commitremap"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.Flags().StringP("mapping-file", "c", "", "Path to the commit map file Example: /path/to/commit-map")
	rootCmd.MarkFlagRequired("mapping-file")

	rootCmd.Flags().StringP("migration-archive", "m", "", "Path to the migration archive Example: /path/to/migration-archive.tar.gz")
	rootCmd.MarkFlagRequired("migration-archive")
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

		archivePath, _ := cmd.Flags().GetString("migration-archive")

		var extractedDir string
		untarAndRetar := strings.HasSuffix(archivePath, ".tar.gz")

		if untarAndRetar {
			// Extract the provided migration archive so we can modify its JSON contents
			extractedDir, err = archive.UnTar(archivePath, "")
			if err != nil {
				log.Fatalf("Error extracting migration archive: %v", err)
			}
		} else {
			// Treat provided path as an already-extracted directory
			extractedDir = archivePath
		}

		if err := commitremap.ProcessFiles(extractedDir, commitremap.DefaultPrefixes(), commitMap); err != nil {
			log.Fatal(err)
		} else {
			// Re-package the modified directory into a new archive
			tarPath, err := archive.ReTar(extractedDir)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("New archive created: %s", tarPath)
			if untarAndRetar {
				// Cleanup extracted directory after successful re-tar
				if err := os.RemoveAll(extractedDir); err != nil {
					log.Printf("Warning: failed to remove extracted directory %s: %v", extractedDir, err)
				}
			}
		}
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
