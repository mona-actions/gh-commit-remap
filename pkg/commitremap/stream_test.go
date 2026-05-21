package commitremap

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarGz creates a .tar.gz at path with the given entries.
func makeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		if e.isDir {
			hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeDir, Mode: 0o755}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			continue
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(e.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// readTarGz returns all entries from a tar.gz.
func readTarGz(t *testing.T, path string) []tarEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	var entries []tarEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		e := tarEntry{name: hdr.Name, isDir: hdr.Typeflag == tar.TypeDir}
		if !e.isDir {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			e.data = string(data)
		}
		entries = append(entries, e)
	}
	return entries
}

type tarEntry struct {
	name  string
	data  string
	isDir bool
}

func TestStreamRemap_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.tar.gz")
	outPath := filepath.Join(dir, "out.tar.gz")

	oldSHA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newSHA := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	commitMap := map[string]string{oldSHA: newSHA}

	entries := []tarEntry{
		{name: "./", isDir: true},
		{name: "./pull_requests_000001.json", data: `{"sha":"` + oldSHA + `"}`},
		{name: "./issues_000002.json", data: `{"ref":"` + oldSHA + `","other":"value"}`},
		{name: "./users_000001.json", data: `{"name":"test"}`},
		{name: "./organizations_000001.json", data: `{"sha":"` + oldSHA + `"}`},
	}
	makeTarGz(t, inPath, entries)

	stats, err := StreamRemap(inPath, outPath, commitMap, []string{"pull_requests", "issues"})
	if err != nil {
		t.Fatalf("StreamRemap: %v", err)
	}

	if stats.FilesScanned != 2 {
		t.Errorf("FilesScanned = %d, want 2", stats.FilesScanned)
	}
	if stats.FilesChanged() != 2 {
		t.Errorf("FilesChanged = %d, want 2", stats.FilesChanged())
	}

	result := readTarGz(t, outPath)
	for _, e := range result {
		switch {
		case e.name == "./pull_requests_000001.json":
			if !strings.Contains(e.data, newSHA) {
				t.Errorf("pull_requests should contain new SHA, got: %s", e.data)
			}
			if strings.Contains(e.data, oldSHA) {
				t.Errorf("pull_requests still contains old SHA")
			}
		case e.name == "./issues_000002.json":
			if !strings.Contains(e.data, newSHA) {
				t.Errorf("issues should contain new SHA, got: %s", e.data)
			}
		case e.name == "./users_000001.json":
			if e.data != `{"name":"test"}` {
				t.Errorf("users should be unchanged, got: %s", e.data)
			}
		case e.name == "./organizations_000001.json":
			// Not in prefixes, should pass through with original SHA
			if !strings.Contains(e.data, oldSHA) {
				t.Errorf("organizations should still contain old SHA (not remapped), got: %s", e.data)
			}
		}
	}
}

func TestStreamRemap_NoMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.tar.gz")
	outPath := filepath.Join(dir, "out.tar.gz")

	commitMap := map[string]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	entries := []tarEntry{
		{name: "./", isDir: true},
		{name: "./users_000001.json", data: `{"name":"test"}`},
	}
	makeTarGz(t, inPath, entries)

	stats, err := StreamRemap(inPath, outPath, commitMap, []string{"pull_requests"})
	if err != nil {
		t.Fatalf("StreamRemap: %v", err)
	}
	if stats.FilesScanned != 0 {
		t.Errorf("FilesScanned = %d, want 0", stats.FilesScanned)
	}
}

func TestStreamRemap_RejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.tar.gz")
	outPath := filepath.Join(dir, "out.tar.gz")

	commitMap := map[string]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	entries := []tarEntry{
		{name: "/etc/passwd", data: "bad"},
	}
	makeTarGz(t, inPath, entries)

	_, err := StreamRemap(inPath, outPath, commitMap, []string{"pull_requests"})
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("unexpected error: %v", err)
	}
	// Output should be cleaned up
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Error("partial output should have been removed")
	}
}

func TestStreamRemap_RejectsParentTraversal(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.tar.gz")
	outPath := filepath.Join(dir, "out.tar.gz")

	commitMap := map[string]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	entries := []tarEntry{
		{name: "../escape.json", data: "bad"},
	}
	makeTarGz(t, inPath, entries)

	_, err := StreamRemap(inPath, outPath, commitMap, []string{"pull_requests"})
	if err == nil {
		t.Fatal("expected error for parent traversal, got nil")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStreamRemap_EmptyCommitMap(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.tar.gz")
	outPath := filepath.Join(dir, "out.tar.gz")

	entries := []tarEntry{{name: "./", isDir: true}}
	makeTarGz(t, inPath, entries)

	_, err := StreamRemap(inPath, outPath, map[string]string{}, []string{"pull_requests"})
	if err == nil {
		t.Fatal("expected error for empty commit map")
	}
}

func TestStreamRemap_PreservesEntryOrder(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.tar.gz")
	outPath := filepath.Join(dir, "out.tar.gz")

	commitMap := map[string]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	names := []string{"./", "./z_000001.json", "./a_000001.json", "./m_000001.json"}
	entries := []tarEntry{
		{name: names[0], isDir: true},
		{name: names[1], data: "z"},
		{name: names[2], data: "a"},
		{name: names[3], data: "m"},
	}
	makeTarGz(t, inPath, entries)

	_, err := StreamRemap(inPath, outPath, commitMap, []string{})
	if err != nil {
		t.Fatalf("StreamRemap: %v", err)
	}

	result := readTarGz(t, outPath)
	for i, e := range result {
		if e.name != names[i] {
			t.Errorf("entry %d: got name %q, want %q", i, e.name, names[i])
		}
	}
}

func TestShouldRemap(t *testing.T) {
	prefixes := map[string]bool{"pull_requests": true, "issues": true}
	tests := []struct {
		name string
		want bool
	}{
		{"./pull_requests_000001.json", true},
		{"./issues_000002.json", true},
		{"./users_000001.json", false},
		{"./pull_requests.json", false},       // no _digits suffix
		{"./pull_requests_abc.json", false},   // non-digit suffix
		{"./subdir/pull_requests_1.json", true}, // nested
		{"./readme.md", false},
		{"pull_requests_1.json", true},
	}
	for _, tt := range tests {
		if got := shouldRemap(tt.name, prefixes); got != tt.want {
			t.Errorf("shouldRemap(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
