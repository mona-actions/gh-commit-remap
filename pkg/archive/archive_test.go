package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func buildTarGz(t *testing.T, entries []tar.Header, contents map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, entry := range entries {
		if entry.Typeflag == 0 {
			entry.Typeflag = tar.TypeReg
		}
		if entry.Typeflag == tar.TypeReg && entry.Size == 0 {
			entry.Size = int64(len(contents[entry.Name]))
		}
		if err := tarWriter.WriteHeader(&entry); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if content, ok := contents[entry.Name]; ok {
			if _, err := tarWriter.Write(content); err != nil {
				t.Fatalf("failed to write tar content: %v", err)
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func writeArchive(t *testing.T, dir, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write archive: %v", err)
	}
	return path
}

func collectListing(t *testing.T, root string) []string {
	t.Helper()

	var entries []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			rel += "/"
		}
		entries = append(entries, rel)
		return nil
	}); err != nil {
		t.Fatalf("failed to collect listing: %v", err)
	}
	sort.Strings(entries)
	return entries
}

func TestReTar(t *testing.T) {
	srcDir := t.TempDir()

	origContent := []byte("test")
	if err := os.WriteFile(filepath.Join(srcDir, "testfile"), origContent, 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	tarFile, err := ReTar(srcDir)
	if err != nil {
		t.Fatalf("ReTar failed: %v", err)
	}
	defer os.Remove(tarFile)

	if _, err := os.Stat(tarFile); err != nil {
		t.Fatalf("tar file was not created: %v", err)
	}

	extractDir := t.TempDir()
	if _, err := UnTar(tarFile, extractDir); err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}

	extractedContent, err := os.ReadFile(filepath.Join(extractDir, "testfile"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}

	if !bytes.Equal(origContent, extractedContent) {
		t.Fatalf("original file content and extracted file content do not match")
	}
}

func TestUnTar(t *testing.T) {
	srcDir := t.TempDir()

	content := []byte("hello world")
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), content, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	archiveName, err := ReTar(srcDir)
	if err != nil {
		t.Fatalf("ReTar failed: %v", err)
	}
	defer os.Remove(archiveName)

	destDir, err := UnTar(archiveName, "")
	if err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}
	defer os.RemoveAll(destDir)

	extractedContent, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if !bytes.Equal(content, extractedContent) {
		t.Fatalf("extracted content mismatch")
	}
}

func TestUnTarErrors(t *testing.T) {
	if _, err := UnTar("does-not-exist.tar.gz", ""); err == nil {
		t.Fatalf("expected error for non-existent archive")
	}

	if _, err := UnTar("", ""); err == nil {
		t.Fatalf("expected error for empty archive path")
	}
}

func TestUnTar_TarSlip(t *testing.T) {
	archivePath := writeArchive(t, t.TempDir(), "evil.tar.gz", buildTarGz(t, []tar.Header{{
		Name: "../../etc/passwd",
		Mode: 0o644,
		Size: int64(len("evil")),
	}}, map[string][]byte{"../../etc/passwd": []byte("evil")}))

	_, err := UnTar(archivePath, filepath.Join(t.TempDir(), "extract"))
	if err == nil || !strings.Contains(err.Error(), "escapes destination") {
		t.Fatalf("expected escapes destination error, got %v", err)
	}
}

func TestUnTar_AbsolutePath(t *testing.T) {
	archivePath := writeArchive(t, t.TempDir(), "evil.tar.gz", buildTarGz(t, []tar.Header{{
		Name: "/tmp/evil",
		Mode: 0o644,
		Size: int64(len("evil")),
	}}, map[string][]byte{"/tmp/evil": []byte("evil")}))

	_, err := UnTar(archivePath, filepath.Join(t.TempDir(), "extract"))
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestUnTar_Symlink(t *testing.T) {
	archivePath := writeArchive(t, t.TempDir(), "symlink.tar.gz", buildTarGz(t, []tar.Header{{
		Name:     "link",
		Mode:     0o777,
		Typeflag: tar.TypeSymlink,
		Linkname: "target",
	}}, nil))

	_, err := UnTar(archivePath, filepath.Join(t.TempDir(), "extract"))
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}

func TestUnTar_NonGzip(t *testing.T) {
	archivePath := writeArchive(t, t.TempDir(), "plain.tar.gz", []byte("not gzip"))

	_, err := UnTar(archivePath, filepath.Join(t.TempDir(), "extract"))
	if err == nil {
		t.Fatalf("expected error for non-gzip archive")
	}
}

func TestUnTar_CorruptGzip(t *testing.T) {
	archivePath := writeArchive(t, t.TempDir(), "corrupt.tar.gz", []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 'g', 'a', 'r', 'b', 'a', 'g', 'e'})

	_, err := UnTar(archivePath, filepath.Join(t.TempDir(), "extract"))
	if err == nil {
		t.Fatalf("expected error for corrupt gzip archive")
	}
}

func TestUnTar_EmptyArchive(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "extract")
	archivePath := writeArchive(t, t.TempDir(), "empty.tar.gz", buildTarGz(t, nil, nil))

	returnedDir, err := UnTar(archivePath, destDir)
	if err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}
	if returnedDir != destDir {
		t.Fatalf("expected returned dir %q, got %q", destDir, returnedDir)
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("failed to read dest dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty dest dir, got %d entries", len(entries))
	}
}

func TestUnTar_PermissionsPreserved(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := ReTarDir(srcDir, archivePath); err != nil {
		t.Fatalf("ReTarDir failed: %v", err)
	}
	destDir := filepath.Join(t.TempDir(), "extract")
	if _, err := UnTar(archivePath, destDir); err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(destDir, "secret.txt"))
	if err != nil {
		t.Fatalf("failed to stat extracted file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected mode 0600, got %o", got)
	}
}

func TestUnTar_DestDirAutoCreated(t *testing.T) {
	archivePath := writeArchive(t, t.TempDir(), "one.tar.gz", buildTarGz(t, []tar.Header{{
		Name: "file.txt",
		Mode: 0o644,
		Size: int64(len("content")),
	}}, map[string][]byte{"file.txt": []byte("content")}))
	destDir := filepath.Join(t.TempDir(), "nested", "path")

	if _, err := UnTar(archivePath, destDir); err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}
	if info, err := os.Stat(destDir); err != nil || !info.IsDir() {
		t.Fatalf("expected dest dir to be created, info=%v err=%v", info, err)
	}
}

func TestUnTar_DotDestDir(t *testing.T) {
	testUnTarRelativeDestDir(t, ".")
}

func TestUnTar_DotSlashDestDir(t *testing.T) {
	testUnTarRelativeDestDir(t, "./")
}

func testUnTarRelativeDestDir(t *testing.T, destDir string) {
	t.Helper()

	cwd := t.TempDir()
	archivePath := writeArchive(t, cwd, "dot.tar.gz", buildTarGz(t, []tar.Header{{
		Name: "issues.json",
		Mode: 0o644,
		Size: int64(len("{}")),
	}, {
		Name:     "nested",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}, {
		Name: "nested/payload.txt",
		Mode: 0o644,
		Size: int64(len("payload")),
	}}, map[string][]byte{
		"issues.json":        []byte("{}"),
		"nested/payload.txt": []byte("payload"),
	}))
	t.Chdir(cwd)

	returnedDir, err := UnTar(archivePath, destDir)
	if err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}
	if returnedDir != destDir {
		t.Fatalf("expected returned dir %q, got %q", destDir, returnedDir)
	}
	if got, err := os.ReadFile(filepath.Join(cwd, "issues.json")); err != nil || string(got) != "{}" {
		t.Fatalf("expected issues.json in cwd, got %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(cwd, "nested", "payload.txt")); err != nil || string(got) != "payload" {
		t.Fatalf("expected nested payload in cwd, got %q err=%v", got, err)
	}
}

func TestUnTar_PreExistingSymlinkInDest(t *testing.T) {
	baseDir := t.TempDir()
	destDir := filepath.Join(baseDir, "dest")
	outsideDir := filepath.Join(baseDir, "outside")
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}
	if err := os.Mkdir(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(destDir, "link")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	archivePath := writeArchive(t, baseDir, "symlink-escape.tar.gz", buildTarGz(t, []tar.Header{{
		Name: "link/payload.txt",
		Mode: 0o644,
		Size: int64(len("escaped")),
	}}, map[string][]byte{"link/payload.txt": []byte("escaped")}))

	if _, err := UnTar(archivePath, destDir); err == nil {
		t.Fatalf("expected UnTar to reject pre-existing symlink in dest")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "payload.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected outside payload not to exist, stat err=%v", err)
	}
}

func TestUnTar_DestDirIsSymlink(t *testing.T) {
	baseDir := t.TempDir()
	targetDir := filepath.Join(baseDir, "target")
	destDir := filepath.Join(baseDir, "dest")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if err := os.Symlink(targetDir, destDir); err != nil {
		t.Fatalf("failed to create dest symlink: %v", err)
	}
	archivePath := writeArchive(t, baseDir, "dest-symlink.tar.gz", buildTarGz(t, []tar.Header{{
		Name: "payload.txt",
		Mode: 0o644,
		Size: int64(len("payload")),
	}}, map[string][]byte{"payload.txt": []byte("payload")}))

	if _, err := UnTar(archivePath, destDir); err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "payload.txt")); err != nil || string(got) != "payload" {
		t.Fatalf("expected payload in symlink target, got %q err=%v", got, err)
	}
}

func TestReTarDir_RoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("top-level"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "inner.txt"), []byte("nested content"), 0o644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := ReTarDir(srcDir, archivePath); err != nil {
		t.Fatalf("ReTarDir failed: %v", err)
	}
	destDir := filepath.Join(t.TempDir(), "extract")
	if _, err := UnTar(archivePath, destDir); err != nil {
		t.Fatalf("UnTar failed: %v", err)
	}

	wantListing := []string{"file.txt", "nested/", "nested/inner.txt"}
	if gotListing := collectListing(t, destDir); !reflect.DeepEqual(gotListing, wantListing) {
		t.Fatalf("listing mismatch: got %v, want %v", gotListing, wantListing)
	}
	for _, rel := range []string{"file.txt", filepath.Join("nested", "inner.txt")} {
		want, err := os.ReadFile(filepath.Join(srcDir, rel))
		if err != nil {
			t.Fatalf("failed to read source %q: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(destDir, rel))
		if err != nil {
			t.Fatalf("failed to read extracted %q: %v", rel, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("content mismatch for %q", rel)
		}
	}
}

func TestReTarDir_OutputInsideSrcDir(t *testing.T) {
	srcDir := t.TempDir()
	outPath := filepath.Join(srcDir, "out.tar.gz")

	err := ReTarDir(srcDir, outPath)
	if err == nil || !strings.Contains(err.Error(), "must not be inside source directory") {
		t.Fatalf("expected output-inside-source error, got %v", err)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected archive not to be created, stat err=%v", statErr)
	}
}

func TestReTarDir_OutputEqualsSrcDir(t *testing.T) {
	srcDir := t.TempDir()

	err := ReTarDir(srcDir, srcDir)
	if err == nil || !strings.Contains(err.Error(), "must not be inside source directory") {
		t.Fatalf("expected output-equals-source error, got %v", err)
	}
}

func TestReTarDir_LegacyLayout(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("failed to write nested source file: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := ReTarDir(srcDir, archivePath); err != nil {
		t.Fatalf("ReTarDir failed: %v", err)
	}

	archiveFile, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer archiveFile.Close()
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	var headers []tar.Header
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read tar header: %v", err)
		}
		headers = append(headers, *header)
	}
	if len(headers) == 0 {
		t.Fatalf("expected archive entries")
	}
	if headers[0].Name != "./" || headers[0].Typeflag != tar.TypeDir {
		t.Fatalf("first entry mismatch: name=%q type=%d", headers[0].Name, headers[0].Typeflag)
	}

	foundSubDir := false
	for _, header := range headers[1:] {
		if !strings.HasPrefix(header.Name, "./") {
			t.Fatalf("entry %q missing ./ prefix", header.Name)
		}
		if header.Name == "./sub/" {
			foundSubDir = true
		}
	}
	if !foundSubDir {
		t.Fatalf("expected ./sub/ directory entry with trailing slash, got %v", headers)
	}
}

func TestReTarDir_ParentDirCreated(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "does", "not", "exist", "out.tar.gz")

	if err := ReTarDir(srcDir, outPath); err != nil {
		t.Fatalf("ReTarDir failed: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected archive to exist: %v", err)
	}
}

func TestReTarDir_SrcMissing(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.tar.gz")

	if err := ReTarDir(filepath.Join(t.TempDir(), "missing"), outPath); err == nil {
		t.Fatalf("expected error for missing source directory")
	}
}

func TestReTarDir_Overwrites(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "out.tar.gz")
	if err := os.WriteFile(outPath, []byte("garbage"), 0o644); err != nil {
		t.Fatalf("failed to pre-create archive: %v", err)
	}

	if err := ReTarDir(srcDir, outPath); err != nil {
		t.Fatalf("ReTarDir failed: %v", err)
	}
	if _, err := UnTar(outPath, filepath.Join(t.TempDir(), "extract")); err != nil {
		t.Fatalf("expected overwritten archive to extract cleanly: %v", err)
	}
}

func TestReTar_LegacyNamingInCWD(t *testing.T) {
	cwd := t.TempDir()
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCWD); err != nil {
			t.Fatalf("failed to restore current directory: %v", err)
		}
	})
	srcDir := filepath.Join(t.TempDir(), "source-dir")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	archiveName, err := ReTar(srcDir)
	if err != nil {
		t.Fatalf("ReTar failed: %v", err)
	}
	wantName := filepath.Base(srcDir) + "-REMAPPED.tar.gz"
	if archiveName != wantName {
		t.Fatalf("expected archive name %q, got %q", wantName, archiveName)
	}
	if _, err := os.Stat(filepath.Join(cwd, wantName)); err != nil {
		t.Fatalf("expected archive in cwd: %v", err)
	}
}
