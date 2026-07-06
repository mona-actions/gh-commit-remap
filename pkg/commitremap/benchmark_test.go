package commitremap

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// generateCommitMap creates a commit map with n entries of 40-char hex SHAs.
func generateCommitMap(n int) map[string]string {
	m := make(map[string]string, n)
	buf := make([]byte, 20) // 20 bytes = 40 hex chars
	for i := 0; i < n; i++ {
		rand.Read(buf)
		old := hex.EncodeToString(buf)
		rand.Read(buf)
		new_ := hex.EncodeToString(buf)
		m[old] = new_
	}
	return m
}

// generateJSONWithSHAs creates a JSON byte slice containing numObjects objects,
// each with shaFields fields containing SHAs from the commit map.
// hitRate controls what fraction of SHAs are in the commit map (0.0-1.0).
func generateJSONWithSHAs(commitMap map[string]string, numObjects, shaFields int, hitRate float64) []byte {
	// Collect some real keys for hits
	keys := make([]string, 0, len(commitMap))
	for k := range commitMap {
		keys = append(keys, k)
		if len(keys) >= numObjects*shaFields {
			break
		}
	}

	buf := make([]byte, 20)
	objects := make([]map[string]interface{}, numObjects)
	hitCount := int(float64(numObjects*shaFields) * hitRate)
	idx := 0
	for i := 0; i < numObjects; i++ {
		obj := map[string]interface{}{
			"id":         i,
			"created_at": "2024-01-15T10:30:00Z",
			"url":        fmt.Sprintf("https://github.com/org/repo/pull/%d", i),
		}
		for f := 0; f < shaFields; f++ {
			fieldName := fmt.Sprintf("sha_%d", f)
			if idx < hitCount && len(keys) > 0 {
				obj[fieldName] = keys[idx%len(keys)]
			} else {
				rand.Read(buf)
				obj[fieldName] = hex.EncodeToString(buf)
			}
			idx++
		}
		objects[i] = obj
	}

	data, _ := json.Marshal(objects)
	return data
}

// writeJSONFixtureFiles creates numFiles JSON files in dir, each containing
// numObjects objects with SHA fields.
func writeJSONFixtureFiles(tb testing.TB, dir string, prefix string, numFiles, numObjects int, commitMap map[string]string) {
	tb.Helper()
	for i := 0; i < numFiles; i++ {
		name := fmt.Sprintf("%s_%06d.json", prefix, i+1)
		data := generateJSONWithSHAs(commitMap, numObjects, 3, 0.5)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			tb.Fatal(err)
		}
	}
}

// ── Benchmarks ───────────────────────────────────────────────────────────────

// BenchmarkReplaceSHABytes measures the core byte-sliding window replacement.
// This is the innermost hot loop — called once per metadata file.
func BenchmarkReplaceSHABytes(b *testing.B) {
	sizes := []struct {
		name      string
		mapSize   int
		jsonObjs  int
		shaFields int
	}{
		{"small-map/small-json", 100, 50, 2},
		{"large-map/small-json", 1_000_000, 50, 2},
		{"large-map/medium-json", 1_000_000, 500, 3},
		{"large-map/large-json", 1_000_000, 5000, 3},
		{"monorepo-scale", 1_834_000, 1000, 4},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			commitMap := generateCommitMap(s.mapSize)
			data := generateJSONWithSHAs(commitMap, s.jsonObjs, s.shaFields, 0.5)
			shaLen := 40

			// Pre-allocate a working buffer to avoid measuring make+copy overhead
			input := make([]byte, len(data))
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				copy(input, data)
				ReplaceSHABytes(input, commitMap, shaLen)
			}
		})
	}
}

// BenchmarkReplaceSHABytes_NoHits measures scanning overhead when no SHAs match.
func BenchmarkReplaceSHABytes_NoHits(b *testing.B) {
	commitMap := generateCommitMap(1_834_000)
	data := generateJSONWithSHAs(commitMap, 1000, 4, 0.0)
	differentMap := generateCommitMap(1_834_000)
	shaLen := 40

	input := make([]byte, len(data))
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		copy(input, data)
		ReplaceSHABytes(input, differentMap, shaLen)
	}
}

// BenchmarkUpdateMetadataFile measures single-file remap (read + replace + write).
func BenchmarkUpdateMetadataFile(b *testing.B) {
	commitMap := generateCommitMap(1_834_000)
	shaLen := 40
	data := generateJSONWithSHAs(commitMap, 500, 3, 0.5)

	dir := b.TempDir()
	filePath := filepath.Join(dir, "test_000001.json")

	b.SetBytes(int64(len(data)))
	for b.Loop() {
		os.WriteFile(filePath, data, 0644)
		updateMetadataFile(filePath, commitMap, shaLen)
	}
}

// BenchmarkParseCommitMap measures parsing a commit-map file.
func BenchmarkParseCommitMap(b *testing.B) {
	sizes := []struct {
		name string
		n    int
	}{
		{"1K", 1_000},
		{"100K", 100_000},
		{"1M", 1_000_000},
		{"1.8M", 1_834_000},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			commitMap := generateCommitMap(s.n)
			dir := b.TempDir()
			filePath := filepath.Join(dir, "commit-map")

			// Write commit-map file
			f, _ := os.Create(filePath)
			fmt.Fprintln(f, "old new")
			for old, new_ := range commitMap {
				fmt.Fprintf(f, "%s %s\n", old, new_)
			}
			f.Close()

			fi, _ := os.Stat(filePath)
			b.SetBytes(fi.Size())
			for b.Loop() {
				ParseCommitMap(filePath)
			}
		})
	}
}

// BenchmarkProcessFiles measures the full parallel pipeline.
func BenchmarkProcessFiles(b *testing.B) {
	configs := []struct {
		name       string
		mapSize    int
		numFiles   int
		objPerFile int
		workers    int
	}{
		{"10-files/8-workers", 100_000, 10, 200, 8},
		{"100-files/8-workers", 100_000, 100, 200, 8},
		{"100-files/16-workers", 100_000, 100, 200, 16},
	}

	for _, c := range configs {
		b.Run(c.name, func(b *testing.B) {
			commitMap := generateCommitMap(c.mapSize)

			// Create fixture dir once
			baseDir := b.TempDir()
			writeJSONFixtureFiles(b, baseDir, "pull_requests", c.numFiles, c.objPerFile, commitMap)

			b.ResetTimer()
			for b.Loop() {
				b.StopTimer()
				writeJSONFixtureFiles(b, baseDir, "pull_requests", c.numFiles, c.objPerFile, commitMap)
				b.StartTimer()

				ProcessFiles(baseDir, []string{"pull_requests"}, commitMap, ProcessOptions{NumWorkers: c.workers})
			}
		})
	}
}

// BenchmarkIsHexByte measures the hex byte check (called per byte in the hot loop).
func BenchmarkIsHexByte(b *testing.B) {
	inputs := []byte("0123456789abcdefABCDEF!@#$%^&*()ghijklmnopqrstuvwxyz")
	for b.Loop() {
		for _, c := range inputs {
			isHexByte(c)
		}
	}
}
