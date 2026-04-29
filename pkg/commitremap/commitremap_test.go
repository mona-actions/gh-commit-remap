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

func TestProcessFiles(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		commitMap := map[string]string{
			"oldSHA1": "newSHA1",
			"oldSHA2": "newSHA2",
			"oldSHA3": "newSHA3",
		}

		fixtures := map[string]struct {
			input string
			want  string
		}{
			"pull_requests_000001.json": {
				input: `{"sha":"oldSHA1","url":"https://example.invalid/oldSHA1","nested":[{"head":"oldSHA2","body":"mention oldSHA3"}],"untouched":"keep"}`,
				want:  `{"sha":"newSHA1","url":"https://example.invalid/oldSHA1","nested":[{"head":"newSHA2","body":"mention oldSHA3"}],"untouched":"keep"}`,
			},
			"issues_000001.json": {
				input: `[{"events":[{"commit_id":"oldSHA2"},{"commit_id":"unknownSHA"}],"title":"oldSHA2 in title"}]`,
				want:  `[{"events":[{"commit_id":"newSHA2"},{"commit_id":"unknownSHA"}],"title":"oldSHA2 in title"}]`,
			},
			"issue_events_000001.json": {
				input: `{"items":[{"payload":{"before":"oldSHA3","after":"oldSHA1"}}],"count":1}`,
				want:  `{"items":[{"payload":{"before":"newSHA3","after":"newSHA1"}}],"count":1}`,
			},
		}

		paths := make(map[string]string, len(fixtures))
		for name, fixture := range fixtures {
			p := filepath.Join(dir, name)
			paths[name] = p
			writeFile(t, p, fixture.input)
		}

		stats, err := ProcessFiles(dir, DefaultPrefixes(), commitMap)
		if err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		for name, fixture := range fixtures {
			assertJSONFileEqual(t, filepath.Join(dir, name), fixture.want)
		}

		if got, want := stats.FilesScanned, 3; got != want {
			t.Fatalf("FilesScanned = %d, want %d", got, want)
		}
		if got, want := stats.FilesChanged(), 3; got != want {
			t.Fatalf("FilesChanged() = %d, want %d", got, want)
		}
		// pull_requests: sha=oldSHA1, nested[0].head=oldSHA2 -> 2
		// issues: events[0].commit_id=oldSHA2 -> 1
		// issue_events: payload.before=oldSHA3, payload.after=oldSHA1 -> 2
		if got, want := stats.TotalReplacements(), 5; got != want {
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
		want := `{"sha":"oldSHA1","nested":[{"sha":"oldSHA2"}]}`
		writeFile(t, filePath, want)

		if _, err := ProcessFiles(dir, []string{"pull_requests"}, map[string]string{}); err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		assertJSONFileEqual(t, filePath, want)
	})

	t.Run("no matching files", func(t *testing.T) {
		if _, err := ProcessFiles(t.TempDir(), DefaultPrefixes(), map[string]string{"oldSHA1": "newSHA1"}); err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}
	})

	t.Run("custom prefixes", func(t *testing.T) {
		dir := t.TempDir()
		fooPath := filepath.Join(dir, "foo_000001.json")
		pullPath := filepath.Join(dir, "pull_requests_000001.json")
		writeFile(t, fooPath, `{"sha":"oldSHA1"}`)
		writeFile(t, pullPath, `{"sha":"oldSHA1"}`)

		stats, err := ProcessFiles(dir, []string{"foo"}, map[string]string{"oldSHA1": "newSHA1"})
		if err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		assertJSONFileEqual(t, fooPath, `{"sha":"newSHA1"}`)
		assertJSONFileEqual(t, pullPath, `{"sha":"oldSHA1"}`)

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
		writeFile(t, filePath, `{"items":[{"sha":"oldSHA1"},{"nested":{"sha":"oldSHA2","children":["oldSHA3","oldSHA4"]}}]}`)

		commitMap := map[string]string{
			"oldSHA1": "newSHA1",
			"oldSHA2": "newSHA2",
			"oldSHA3": "newSHA3",
			"oldSHA4": "newSHA4",
		}
		if _, err := ProcessFiles(dir, []string{"pull_requests"}, commitMap); err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		assertJSONFileEqual(t, filePath, `{"items":[{"sha":"newSHA1"},{"nested":{"sha":"newSHA2","children":["newSHA3","newSHA4"]}}]}`)
	})

	t.Run("non matching strings unchanged", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "issues_000001.json")
		original := `{"title":"no SHA here","body":"https://example.invalid/oldSHA1","labels":["bug","help wanted"]}`
		writeFile(t, filePath, original)

		if _, err := ProcessFiles(dir, []string{"issues"}, map[string]string{"oldSHA1": "newSHA1"}); err != nil {
			t.Fatalf("ProcessFiles returned error: %v", err)
		}

		assertJSONFileEqual(t, filePath, original)
	})
}

func TestDefaultPrefixes(t *testing.T) {
	want := []string{"pull_requests", "issues", "issue_events"}
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
	original := []byte(`{"sha":"someSHA","nested":[{"sha":"otherSHA"}]}`)
	if err := os.WriteFile(filePath, original, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	infoBefore, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	stats, err := ProcessFiles(dir, []string{"pull_requests"}, map[string]string{"unrelated": "x"})
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

func TestProcessFiles_ReturnsPartialStatsOnError(t *testing.T) {
	dir := t.TempDir()
	// filepath.Glob returns sorted results, so pull_requests_000001.json is processed before pull_requests_000002.json.
	validPath := filepath.Join(dir, "pull_requests_000001.json")
	badPath := filepath.Join(dir, "pull_requests_000002.json")
	writeFile(t, validPath, `{"sha":"oldSHA1"}`)
	writeFile(t, badPath, `{not valid json`)

	stats, err := ProcessFiles(dir, []string{"pull_requests"}, map[string]string{"oldSHA1": "newSHA1"})
	if err == nil {
		t.Fatal("expected error from malformed JSON file")
	}
	if stats.FilesScanned != 2 {
		t.Fatalf("FilesScanned = %d, want 2", stats.FilesScanned)
	}
	if len(stats.PerFile) < 1 {
		t.Fatalf("len(PerFile) = %d, want >= 1", len(stats.PerFile))
	}
	if _, ok := stats.PerFile[validPath]; !ok {
		t.Fatalf("PerFile must contain validPath %q; got %#v", validPath, stats.PerFile)
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
