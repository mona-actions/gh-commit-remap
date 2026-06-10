/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mona-actions/gh-commit-remap/pkg/archive"
	"github.com/mona-actions/gh-commit-remap/pkg/commitremap"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.Flags().StringP("mapping-file", "c", "", "Path to the commit map file Example: /path/to/commit-map")
	rootCmd.MarkFlagRequired("mapping-file")

	rootCmd.Flags().StringP("migration-archive", "m", "", "Path to the migration archive Example: /path/to/migration-archive.tar.gz")
	rootCmd.MarkFlagRequired("migration-archive")

	rootCmd.Flags().IntP("threads", "t", 0, "Number of parallel goroutines for metadata processing (default: number of CPUs)")

	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
}

func renderSummaryTable(stats commitremap.Stats, extractedDir string) {
	if stats.FilesChanged() == 0 {
		pterm.Info.Println("No files were modified — none of the SHAs in the archive matched the commit-map.")
		return
	}

	type row struct {
		rel   string
		count int
	}
	rows := make([]row, 0, len(stats.PerFile))
	for absPath, count := range stats.PerFile {
		rel, err := filepath.Rel(extractedDir, absPath)
		if err != nil {
			rel = filepath.Base(absPath)
		}
		rows = append(rows, row{rel: rel, count: count})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].rel < rows[j].rel })

	data := pterm.TableData{{"File", "SHAs rewritten"}}
	for _, r := range rows {
		data = append(data, []string{r.rel, fmt.Sprintf("%d", r.count)})
	}
	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		pterm.Warning.Printfln("failed to render summary table: %v", err)
	}
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-commit-remap",
	Short: "remaps commit hashes in a GitHub archive",
	Long: `Is a CLI tool that can remap commits hashed 
	after performing a history re-write when performing a migration For exam`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mapPath, _ := cmd.Flags().GetString("mapping-file")
		commitMap, err := commitremap.ParseCommitMap(mapPath)
		if err != nil {
			return fmt.Errorf("parsing commit map: %w", err)
		}

		archivePath, _ := cmd.Flags().GetString("migration-archive")

		pterm.DefaultSection.Println("Remap")
		spinner, _ := pterm.DefaultSpinner.Start("Remapping SHAs...")

		var (
			stats      commitremap.Stats
			summaryDir string
			outPath    string
		)

		if strings.HasSuffix(archivePath, ".tar.gz") {
			// Streaming path: remap in-flight without extracting to disk
			outPath = strings.TrimSuffix(filepath.Base(archivePath), ".tar.gz") + "-REMAPPED.tar.gz"

			stats, err = archive.StreamRemap(archivePath, outPath, commitMap, commitremap.DefaultPrefixes())
			if err != nil {
				spinner.Fail("Stream remap failed")
				return fmt.Errorf("stream remap: %w", err)
			}
		} else {
			// Directory path: remap files in-place on disk
			threads, _ := cmd.Flags().GetInt("threads")
			summaryDir = archivePath

			stats, err = commitremap.ProcessFiles(archivePath, commitremap.DefaultPrefixes(), commitMap, commitremap.ProcessOptions{NumWorkers: threads})
			if err != nil {
				spinner.Fail("Remap failed")
				renderSummaryTable(stats, archivePath)
				return fmt.Errorf("remapping SHAs: %w", err)
			}
		}

		spinner.Success(fmt.Sprintf("Remapped %d SHAs across %d files (scanned %d)", stats.TotalReplacements(), stats.FilesChanged(), stats.FilesScanned))
		renderSummaryTable(stats, summaryDir)
		if outPath != "" {
			pterm.Success.Printfln("New archive created: %s", outPath)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
}
