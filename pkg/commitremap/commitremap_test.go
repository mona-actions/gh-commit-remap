package commitremap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCommitMap(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expected    map[string]string
		errContains string
	}{
		{
			name: "happy path",
			content: "oldSHA1 newSHA1\n" +
				"oldSHA2 newSHA2\n" +
				"oldSHA3 newSHA3",
			expected: map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
				"oldSHA3": "newSHA3",
			},
		},
		{
			name:     "empty file",
			content:  "",
			expected: map[string]string{},
		},
		{
			name: "whitespace-only lines mid-file",
			content: "oldSHA1 newSHA1\n" +
				"  \t  \n" +
				"oldSHA2 newSHA2",
			expected: map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
			},
		},
		{
			name: "CRLF and mixed line endings",
			content: "oldSHA1 newSHA1\r\n" +
				"oldSHA2 newSHA2\n" +
				"oldSHA3 newSHA3\r\n",
			expected: map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
				"oldSHA3": "newSHA3",
			},
		},
		{
			name: "trailing newline",
			content: "oldSHA1 newSHA1\n" +
				"oldSHA2 newSHA2\n",
			expected: map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
			},
		},
		{
			name: "blank lines mid-file",
			content: "oldSHA1 newSHA1\n" +
				"\n" +
				"oldSHA2 newSHA2",
			expected: map[string]string{
				"oldSHA1": "newSHA1",
				"oldSHA2": "newSHA2",
			},
		},
		{
			name: "duplicate old SHA last entry wins",
			content: "oldSHA1 newSHA1\n" +
				"oldSHA1 newerSHA1",
			expected: map[string]string{
				"oldSHA1": "newerSHA1",
			},
		},
		{
			name:        "malformed line with one field",
			content:     "oldSHA1 newSHA1\noffendingLine\noldSHA2 newSHA2",
			errContains: "offendingLine",
		},
		{
			name:        "malformed line with three fields",
			content:     "oldSHA1 newSHA1\noldSHA2 newSHA2 extra\noldSHA3 newSHA3",
			errContains: "oldSHA2 newSHA2 extra",
		},
		{
			name: "skips git-filter-repo header line",
			content: "old new\n" +
				"abc123 def456\n" +
				"ghi789 jkl012",
			expected: map[string]string{
				"abc123": "def456",
				"ghi789": "jkl012",
			},
		},
		{
			name: "header with trailing CRLF",
			content: "old new\r\n" +
				"abc123 def456\r\n",
			expected: map[string]string{
				"abc123": "def456",
			},
		},
		{
			name: "old new as data when not on first line",
			content: "abc123 def456\n" +
				"old new",
			expected: map[string]string{
				"abc123": "def456",
				"old":    "new",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "commit-map")
			if err := os.WriteFile(filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write commit map fixture: %v", err)
			}

			result, err := ParseCommitMap(filePath)
			if tt.errContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.errContains)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error %q to contain %q", err.Error(), tt.errContains)
				}

				var lineErr invalidCommitMapLineError
				if !errors.As(err, &lineErr) {
					t.Fatalf("expected error to wrap invalidCommitMapLineError, got %T: %v", err, err)
				}
				if lineErr.line != tt.errContains {
					t.Fatalf("wrapped line = %q, want %q", lineErr.line, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseCommitMap returned error: %v", err)
			}
			if result == nil {
				t.Fatal("ParseCommitMap returned nil map")
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("ParseCommitMap returned %#v, want %#v", result, tt.expected)
			}
		})
	}

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ParseCommitMap(filepath.Join(t.TempDir(), "missing-commit-map"))
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})
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
		if got := ShouldRemap(tt.name, prefixes); got != tt.want {
			t.Errorf("ShouldRemap(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsHexByte(t *testing.T) {
	valid := "0123456789abcdefABCDEF"
	for _, b := range []byte(valid) {
		if !isHexByte(b) {
			t.Fatalf("isHexByte(%q) = false, want true", b)
		}
	}
	invalid := "ghijklGHIJKL!@#$%^&*() \t\n{}\"/:"
	for _, b := range []byte(invalid) {
		if isHexByte(b) {
			t.Fatalf("isHexByte(%q) = true, want false", b)
		}
	}
}

func TestCommitMapSHALen(t *testing.T) {
	tests := []struct {
		name        string
		commitMap   map[string]string
		wantLen     int
		errContains string
	}{
		{
			name:      "40-char SHAs",
			commitMap: map[string]string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			wantLen:   40,
		},
		{
			name:        "empty map",
			commitMap:   map[string]string{},
			errContains: "empty",
		},
		{
			name:        "inconsistent lengths",
			commitMap:   map[string]string{"aabb": "ccdd", "aabbcc": "ddeeff"},
			errContains: "inconsistent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CommitMapSHALen(tt.commitMap)
			if tt.errContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.errContains)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantLen {
				t.Fatalf("CommitMapSHALen = %d, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestReplaceSHABytes(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		commitMap map[string]string
		shaLen    int
		wantOut   string
		wantCount int
	}{
		{
			name:      "exact SHA replaced",
			input:     `{"sha":"aabbccdd"}`,
			commitMap: map[string]string{"aabbccdd": "11223344"},
			shaLen:    8,
			wantOut:   `{"sha":"11223344"}`,
			wantCount: 1,
		},
		{
			name:      "SHA in URL replaced",
			input:     `{"url":"https://example.com/commit/aabbccdd/details"}`,
			commitMap: map[string]string{"aabbccdd": "11223344"},
			shaLen:    8,
			wantOut:   `{"url":"https://example.com/commit/11223344/details"}`,
			wantCount: 1,
		},
		{
			name:      "SHA in markdown replaced",
			input:     `{"body":"Fixed in aabbccdd, see also eeff0011"}`,
			commitMap: map[string]string{"aabbccdd": "11223344", "eeff0011": "55667788"},
			shaLen:    8,
			wantOut:   `{"body":"Fixed in 11223344, see also 55667788"}`,
			wantCount: 2,
		},
		{
			name:      "non-hex byte breaks window",
			input:     `aabbXccdd`,
			commitMap: map[string]string{"aabbccdd": "11223344"},
			shaLen:    8,
			wantOut:   `aabbXccdd`,
			wantCount: 0,
		},
		{
			name:      "no match leaves data unchanged",
			input:     `{"sha":"aabbccdd"}`,
			commitMap: map[string]string{"11223344": "55667788"},
			shaLen:    8,
			wantOut:   `{"sha":"aabbccdd"}`,
			wantCount: 0,
		},
		{
			name:      "adjacent SHAs both replaced",
			input:     `aabbccddeeff0011`,
			commitMap: map[string]string{"aabbccdd": "11111111", "eeff0011": "22222222"},
			shaLen:    8,
			wantOut:   `1111111122222222`,
			wantCount: 2,
		},
		{
			name:      "40-char SHA replacement",
			input:     `commit aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa done`,
			commitMap: map[string]string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			shaLen:    40,
			wantOut:   `commit bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb done`,
			wantCount: 1,
		},
		{
			name:      "multiple occurrences of same SHA",
			input:     `aabbccdd and aabbccdd`,
			commitMap: map[string]string{"aabbccdd": "11223344"},
			shaLen:    8,
			wantOut:   `11223344 and 11223344`,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.input)
			out, count := ReplaceSHABytes(data, tt.commitMap, tt.shaLen)
			if string(out) != tt.wantOut {
				t.Fatalf("output = %q, want %q", string(out), tt.wantOut)
			}
			if count != tt.wantCount {
				t.Fatalf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

// Use 40-char hex SHAs for ProcessFiles tests since commitMapSHALen validates.
var testCommitMap = map[string]string{
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "1111111111111111111111111111111111111111",
	"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": "2222222222222222222222222222222222222222",
	"cccccccccccccccccccccccccccccccccccccccc": "3333333333333333333333333333333333333333",
}

func TestProcessFiles(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()

		fixtures := map[string]struct {
			input string
			want  string
		}{
			"pull_requests_000001.json": {
				input: `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","url":"https://example.invalid/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","nested":[{"head":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","body":"mention cccccccccccccccccccccccccccccccccccccccc"}],"untouched":"keep"}`,
				want:  `{"sha":"1111111111111111111111111111111111111111","url":"https://example.invalid/2222222222222222222222222222222222222222","nested":[{"head":"2222222222222222222222222222222222222222","body":"mention 3333333333333333333333333333333333333333"}],"untouched":"keep"}`,
			},
			"issues_000001.json": {
				input: `[{"events":[{"commit_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},{"commit_id":"dddddddddddddddddddddddddddddddddddddddd"}],"title":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb in title"}]`,
				want:  `[{"events":[{"commit_id":"2222222222222222222222222222222222222222"},{"commit_id":"dddddddddddddddddddddddddddddddddddddddd"}],"title":"2222222222222222222222222222222222222222 in title"}]`,
			},
			"issue_events_000001.json": {
				input: `{"items":[{"payload":{"before":"cccccccccccccccccccccccccccccccccccccccc","after":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}],"count":1}`,
				want:  `{"items":[{"payload":{"before":"3333333333333333333333333333333333333333","after":"1111111111111111111111111111111111111111"}}],"count":1}`,
			},
		}

		paths := make(map[string]string, len(fixtures))
		for name, fixture := range fixtures {
			p := filepath.Join(dir, name)
			paths[name] = p
			writeFile(t, p, fixture.input)
		}

		stats, err := ProcessFiles(dir, DefaultPrefixes(), testCommitMap, ProcessOptions{})
		if err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		for name, fixture := range fixtures {
			got, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if string(got) != fixture.want {
				t.Fatalf("file %s = %q, want %q", name, string(got), fixture.want)
			}
		}

		if got, want := stats.FilesScanned, 3; got != want {
			t.Fatalf("FilesScanned = %d, want %d", got, want)
		}
		if got, want := stats.FilesChanged(), 3; got != want {
			t.Fatalf("FilesChanged() = %d, want %d", got, want)
		}
		// pull_requests: sha + url + nested.head + nested.body = 4
		// issues: events[0].commit_id + title = 2
		// issue_events: payload.before + payload.after = 2
		if got, want := stats.TotalReplacements(), 8; got != want {
			t.Fatalf("TotalReplacements() = %d, want %d", got, want)
		}
		for name := range fixtures {
			if n := stats.PerFile[paths[name]]; n <= 0 {
				t.Fatalf("stats.PerFile[%s] = %d, want > 0", paths[name], n)
			}
		}
	})

	t.Run("empty commit map", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "pull_requests_000001.json")
		want := `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","nested":[{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`
		writeFile(t, filePath, want)

		_, err := ProcessFiles(dir, []string{"pull_requests"}, map[string]string{}, ProcessOptions{})
		if err == nil {
			t.Fatal("expected error for empty commit map")
		}
	})

	t.Run("no matching files", func(t *testing.T) {
		_, err := ProcessFiles(t.TempDir(), DefaultPrefixes(), testCommitMap, ProcessOptions{})
		if err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}
	})

	t.Run("custom prefixes", func(t *testing.T) {
		dir := t.TempDir()
		fooPath := filepath.Join(dir, "foo_000001.json")
		pullPath := filepath.Join(dir, "pull_requests_000001.json")
		writeFile(t, fooPath, `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
		writeFile(t, pullPath, `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)

		stats, err := ProcessFiles(dir, []string{"foo"}, testCommitMap, ProcessOptions{})
		if err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		got, _ := os.ReadFile(fooPath)
		if !strings.Contains(string(got), "1111111111111111111111111111111111111111") {
			t.Fatalf("foo file should have SHA replaced, got %s", string(got))
		}
		got2, _ := os.ReadFile(pullPath)
		if strings.Contains(string(got2), "1111111111111111111111111111111111111111") {
			t.Fatal("pull_requests file should NOT have been processed with custom prefix")
		}

		if got, want := len(stats.PerFile), 1; got != want {
			t.Fatalf("len(stats.PerFile) = %d, want %d", got, want)
		}
		if n, ok := stats.PerFile[fooPath]; !ok || n <= 0 {
			t.Fatalf("stats.PerFile[fooPath] = %d, ok=%v; want >0 entry", n, ok)
		}
		if _, ok := stats.PerFile[pullPath]; ok {
			t.Fatalf("stats.PerFile must not contain pullPath")
		}
	})

	t.Run("single-pass behavior remaps all keys", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "pull_requests_000001.json")
		writeFile(t, filePath, `{"items":[{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"nested":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","children":["cccccccccccccccccccccccccccccccccccccccc"]}}]}`)

		if _, err := ProcessFiles(dir, []string{"pull_requests"}, testCommitMap, ProcessOptions{}); err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		got, _ := os.ReadFile(filePath)
		gotStr := string(got)
		for _, expected := range []string{"1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222", "3333333333333333333333333333333333333333"} {
			if !strings.Contains(gotStr, expected) {
				t.Fatalf("expected %s in output, got %s", expected, gotStr)
			}
		}
	})

	t.Run("SHAs in URLs and markdown are now replaced", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "issues_000001.json")
		input := `{"title":"no SHA here","body":"https://example.invalid/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","labels":["bug","help wanted"]}`
		writeFile(t, filePath, input)

		stats, err := ProcessFiles(dir, []string{"issues"}, testCommitMap, ProcessOptions{})
		if err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		got, _ := os.ReadFile(filePath)
		if !strings.Contains(string(got), "1111111111111111111111111111111111111111") {
			t.Fatalf("SHA in URL should be replaced, got %s", string(got))
		}
		if stats.TotalReplacements() != 1 {
			t.Fatalf("TotalReplacements() = %d, want 1", stats.TotalReplacements())
		}
	})
}

func TestDefaultPrefixes(t *testing.T) {
	want := []string{
		"issues",
		"issue_events",
		"issue_comments",
		"pull_requests",
		"pull_request_reviews",
		"pull_request_review_comments",
		"pull_request_review_threads",
		"commit_comments",
	}
	got := DefaultPrefixes()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultPrefixes() = %#v, want %#v", got, want)
	}

	// Mutating the returned slice must not affect later calls.
	got[0] = "mutated"
	fresh := DefaultPrefixes()
	if !reflect.DeepEqual(fresh, want) {
		t.Fatalf("DefaultPrefixes() returned shared state; got %#v after caller mutation", fresh)
	}
}

func TestProcessFiles_SkipsWriteWhenNoReplacements(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "pull_requests_000001.json")
	original := []byte(`{"sha":"dddddddddddddddddddddddddddddddddddddddd","nested":[{"sha":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}]}`)
	if err := os.WriteFile(filePath, original, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	infoBefore, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	// commitMap doesn't contain the SHAs in the file
	noMatchMap := map[string]string{
		"ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00": "1100110011001100110011001100110011001100",
	}
	stats, err := ProcessFiles(dir, []string{"pull_requests"}, noMatchMap, ProcessOptions{})
	if err != nil {
		t.Fatalf("ProcessFiles returned error: %v", err)
	}

	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("file bytes changed; got %q, want %q", got, original)
	}

	infoAfter, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("mtime changed: before=%v after=%v", infoBefore.ModTime(), infoAfter.ModTime())
	}

	if stats.FilesScanned != 1 {
		t.Fatalf("FilesScanned = %d, want 1", stats.FilesScanned)
	}
	if stats.FilesChanged() != 0 {
		t.Fatalf("FilesChanged() = %d, want 0", stats.FilesChanged())
	}
	if len(stats.PerFile) != 0 {
		t.Fatalf("len(PerFile) = %d, want 0", len(stats.PerFile))
	}
}

func TestStats_HelperMethods(t *testing.T) {
	s := Stats{FilesScanned: 5, PerFile: map[string]int{"a": 2, "b": 3}}
	if got, want := s.FilesChanged(), 2; got != want {
		t.Fatalf("FilesChanged() = %d, want %d", got, want)
	}
	if got, want := s.TotalReplacements(), 5; got != want {
		t.Fatalf("TotalReplacements() = %d, want %d", got, want)
	}

	var zero Stats
	if got := zero.FilesChanged(); got != 0 {
		t.Fatalf("zero.FilesChanged() = %d, want 0", got)
	}
	if got := zero.TotalReplacements(); got != 0 {
		t.Fatalf("zero.TotalReplacements() = %d, want 0", got)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func assertJSONFileEqual(t *testing.T, path string, want string) {
	t.Helper()

	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var gotJSON interface{}
	if err := json.Unmarshal(gotBytes, &gotJSON); err != nil {
		t.Fatalf("unmarshal got JSON from %s: %v\n%s", path, err, string(gotBytes))
	}

	var wantJSON interface{}
	if err := json.Unmarshal([]byte(want), &wantJSON); err != nil {
		t.Fatalf("unmarshal want JSON: %v\n%s", err, want)
	}

	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("JSON in %s = %#v, want %#v", path, gotJSON, wantJSON)
	}
}
