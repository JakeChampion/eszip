package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "eszip-cli-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	binary = filepath.Join(dir, "eszip")
	out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to resolve project root: %v", err)
	}
	return dir
}

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", name)
}

func execBinary(t *testing.T, args []string, stdin []byte) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func listFilesRecursive(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}
	return files
}

func TestExtract(t *testing.T) {
	archivePath := testdataPath(t, "redirect.eszip2")
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read test archive: %v", err)
	}

	wantFiles := []string{"main.ts", "a.ts", "b.ts"}

	tests := []struct {
		name  string
		args  []string
		stdin []byte
	}{
		{"file_arg", []string{archivePath}, nil},
		{"stdin_no_arg", nil, archiveData},
		{"stdin_dash", []string{"-"}, archiveData},
	}

	// Create output dirs at parent scope so they survive across subtests.
	dirs := make([]string, len(tests))
	for i := range tests {
		dirs[i] = t.TempDir()
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"extract", "-o", dirs[i]}, tt.args...)
			stdout, stderr, err := execBinary(t, args, tt.stdin)
			if err != nil {
				t.Fatalf("extract failed: %v\nstderr: %s", err, stderr)
			}
			if !strings.Contains(stdout, "Extracted:") {
				t.Error("expected 'Extracted:' in stdout")
			}

			entries := listFilesRecursive(t, dirs[i])
			for _, want := range wantFiles {
				found := false
				for _, e := range entries {
					if strings.HasSuffix(e, "/"+want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing %s in extracted files: %v", want, entries)
				}
			}
		})
	}

	// All input modes should produce identical output.
	t.Run("all_modes_match", func(t *testing.T) {
		refFiles := listFilesRecursive(t, dirs[0])
		for _, dir := range dirs[1:] {
			gotFiles := listFilesRecursive(t, dir)
			if len(refFiles) != len(gotFiles) {
				t.Fatalf("file count mismatch: %d vs %d", len(refFiles), len(gotFiles))
			}
			for i, ref := range refFiles {
				relRef, _ := filepath.Rel(dirs[0], ref)
				relGot, _ := filepath.Rel(dir, gotFiles[i])
				if relRef != relGot {
					t.Errorf("path mismatch: %s vs %s", relRef, relGot)
					continue
				}
				refContent, _ := os.ReadFile(ref)
				gotContent, _ := os.ReadFile(gotFiles[i])
				if !bytes.Equal(refContent, gotContent) {
					t.Errorf("content mismatch for %s", relRef)
				}
			}
		}
	})
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin []byte
	}{
		{"nonexistent_file", []string{"extract", "/nonexistent/archive.eszip2"}, nil},
		{"invalid_stdin", []string{"extract"}, []byte("not a valid eszip")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := execBinary(t, tt.args, tt.stdin)
			if err == nil {
				t.Fatal("expected non-zero exit")
			}
		})
	}
}

func TestView(t *testing.T) {
	stdout, stderr, err := execBinary(t, []string{"view", testdataPath(t, "redirect.eszip2")}, nil)
	if err != nil {
		t.Fatalf("view failed: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{"Specifier:", "Kind:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in view output", want)
		}
	}
}

func TestInfo(t *testing.T) {
	stdout, stderr, err := execBinary(t, []string{"info", testdataPath(t, "redirect.eszip2")}, nil)
	if err != nil {
		t.Fatalf("info failed: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{"Format:", "Modules:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in info output", want)
		}
	}
}

func TestCreate(t *testing.T) {
	outDir := t.TempDir()
	outputPath := filepath.Join(outDir, "test.eszip2")

	jsFile := filepath.Join(outDir, "hello.js")
	if err := os.WriteFile(jsFile, []byte("console.log('hello');\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	stdout, stderr, err := execBinary(t, []string{"create", "-o", outputPath, jsFile}, nil)
	if err != nil {
		t.Fatalf("create failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Created:") {
		t.Error("expected 'Created:' in output")
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestHelp(t *testing.T) {
	stdout, _, err := execBinary(t, []string{"help"}, nil)
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Error("expected 'Usage:' in help output")
	}
}

func TestErrorCases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown_command", []string{"bogus"}},
		{"no_args", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := execBinary(t, tt.args, nil)
			if err == nil {
				t.Fatal("expected non-zero exit")
			}
		})
	}
}
