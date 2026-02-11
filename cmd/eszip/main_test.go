package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func newTestApp() (*app, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return &app{stdout: &stdout, stderr: &stderr, stdin: strings.NewReader("")}, &stdout
}

func newTestAppWithStdin(stdin []byte) (*app, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return &app{stdout: &stdout, stderr: &stderr, stdin: bytes.NewReader(stdin)}, &stdout
}

// run executes the CLI with the given args and returns an error if the command fails.
func (a *app) run(args []string) error {
	cmd := a.rootCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
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

	dirs := make([]string, len(tests))
	for i := range tests {
		dirs[i] = t.TempDir()
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"extract", "-o", dirs[i]}
			args = append(args, tt.args...)

			var a *app
			var stdout *bytes.Buffer
			if tt.stdin != nil {
				a, stdout = newTestAppWithStdin(tt.stdin)
			} else {
				a, stdout = newTestApp()
			}

			if err := a.run(args); err != nil {
				t.Fatalf("extract failed: %v", err)
			}

			if !strings.Contains(stdout.String(), "Extracted:") {
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
	t.Run("nonexistent_file", func(t *testing.T) {
		a, _ := newTestApp()
		err := a.run([]string{"extract", "/nonexistent/archive.eszip2"})
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})

	t.Run("invalid_stdin", func(t *testing.T) {
		a, _ := newTestAppWithStdin([]byte("not a valid eszip"))
		err := a.run([]string{"extract"})
		if err == nil {
			t.Fatal("expected error for invalid input")
		}
	})
}

func TestView(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"view", testdataPath(t, "redirect.eszip2")}); err != nil {
		t.Fatalf("view failed: %v", err)
	}
	for _, want := range []string{"Specifier:", "Kind:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected %q in view output", want)
		}
	}
}

func TestViewWithSpecifier(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"view", "-s", "file:///main.ts", testdataPath(t, "redirect.eszip2")}); err != nil {
		t.Fatalf("view failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "file:///main.ts") || !strings.Contains(out, "Specifier:") {
		t.Error("expected filtered view output for file:///main.ts")
	}
}

func TestViewWithSourceMap(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"view", "-m", testdataPath(t, "redirect.eszip2")}); err != nil {
		t.Fatalf("view failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Source Map") {
		t.Error("expected source map in view output with -m flag")
	}
}

func TestInfo(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"info", testdataPath(t, "redirect.eszip2")}); err != nil {
		t.Fatalf("info failed: %v", err)
	}
	for _, want := range []string{"Format:", "Modules:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected %q in info output", want)
		}
	}
}

func TestInfoV1(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"info", testdataPath(t, "basic.json")}); err != nil {
		t.Fatalf("info failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "V1") {
		t.Error("expected 'V1' in info output for JSON archive")
	}
}

func TestCreate(t *testing.T) {
	outDir := t.TempDir()
	outputPath := filepath.Join(outDir, "test.eszip2")

	jsFile := filepath.Join(outDir, "hello.js")
	if err := os.WriteFile(jsFile, []byte("console.log('hello');\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	a, stdout := newTestApp()
	if err := a.run([]string{"create", "-o", outputPath, jsFile}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Created:") {
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

func TestCreateChecksumOptions(t *testing.T) {
	for _, cs := range []string{"none", "sha256", "xxhash3"} {
		t.Run(cs, func(t *testing.T) {
			outDir := t.TempDir()
			outputPath := filepath.Join(outDir, "test.eszip2")

			jsFile := filepath.Join(outDir, "hello.js")
			if err := os.WriteFile(jsFile, []byte("test"), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			a, _ := newTestApp()
			if err := a.run([]string{"create", "--checksum", cs, "-o", outputPath, jsFile}); err != nil {
				t.Fatalf("create with checksum %s failed: %v", cs, err)
			}

			info, err := os.Stat(outputPath)
			if err != nil {
				t.Fatalf("output file not found: %v", err)
			}
			if info.Size() == 0 {
				t.Error("output file is empty")
			}
		})
	}
}

func TestHelp(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"help"}); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "eszip") {
		t.Error("expected 'eszip' in help output")
	}
}

func TestRunErrors(t *testing.T) {
	t.Run("unknown_command", func(t *testing.T) {
		a, _ := newTestApp()
		if err := a.run([]string{"bogus"}); err == nil {
			t.Fatal("expected error for unknown command")
		}
	})
}

func TestCreateJsonFile(t *testing.T) {
	outDir := t.TempDir()
	outputPath := filepath.Join(outDir, "test.eszip2")

	jsonFile := filepath.Join(outDir, "config.json")
	if err := os.WriteFile(jsonFile, []byte(`{"key":"value"}`), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	a, stdout := newTestApp()
	if err := a.run([]string{"create", "-o", outputPath, jsonFile}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Added:") {
		t.Error("expected 'Added:' in output")
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestCreateWasmFile(t *testing.T) {
	outDir := t.TempDir()
	outputPath := filepath.Join(outDir, "test.eszip2")

	wasmFile := filepath.Join(outDir, "module.wasm")
	// Minimal wasm magic bytes
	if err := os.WriteFile(wasmFile, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	a, stdout := newTestApp()
	if err := a.run([]string{"create", "-o", outputPath, wasmFile}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Added:") {
		t.Error("expected 'Added:' in output")
	}
}

func TestCreateMultipleFiles(t *testing.T) {
	outDir := t.TempDir()
	outputPath := filepath.Join(outDir, "test.eszip2")

	jsFile := filepath.Join(outDir, "main.js")
	jsonFile := filepath.Join(outDir, "data.json")
	wasmFile := filepath.Join(outDir, "mod.wasm")

	if err := os.WriteFile(jsFile, []byte("console.log('hi')"), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if err := os.WriteFile(jsonFile, []byte(`{"a":1}`), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if err := os.WriteFile(wasmFile, []byte{0x00, 0x61, 0x73, 0x6d}, 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	a, stdout := newTestApp()
	if err := a.run([]string{"create", "-o", outputPath, jsFile, jsonFile, wasmFile}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	out := stdout.String()
	if strings.Count(out, "Added:") != 3 {
		t.Errorf("expected 3 'Added:' lines, got %d", strings.Count(out, "Added:"))
	}
	if !strings.Contains(out, "Created:") {
		t.Error("expected 'Created:' in output")
	}
}

func TestCreateInvalidChecksum(t *testing.T) {
	outDir := t.TempDir()
	jsFile := filepath.Join(outDir, "test.js")
	if err := os.WriteFile(jsFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	a, _ := newTestApp()
	err := a.run([]string{"create", "--checksum", "invalid", "-o", filepath.Join(outDir, "out.eszip2"), jsFile})
	if err == nil {
		t.Fatal("expected error for invalid checksum")
	}
}

func TestCreateNonexistentInput(t *testing.T) {
	a, _ := newTestApp()
	err := a.run([]string{"create", "-o", "/tmp/out.eszip2", "/nonexistent/file.js"})
	if err == nil {
		t.Fatal("expected error for nonexistent input file")
	}
}

func TestViewNonexistentFile(t *testing.T) {
	a, _ := newTestApp()
	err := a.run([]string{"view", "/nonexistent/archive.eszip2"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestInfoNonexistentFile(t *testing.T) {
	a, _ := newTestApp()
	err := a.run([]string{"info", "/nonexistent/archive.eszip2"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestViewFilterNonexistentSpecifier(t *testing.T) {
	a, stdout := newTestApp()
	if err := a.run([]string{"view", "-s", "file:///nonexistent.ts", testdataPath(t, "redirect.eszip2")}); err != nil {
		t.Fatalf("view failed: %v", err)
	}
	// Should produce no output for nonexistent specifier
	if strings.Contains(stdout.String(), "Specifier:") {
		t.Error("expected no specifier output for nonexistent filter")
	}
}

func TestCreateThenExtractRoundtrip(t *testing.T) {
	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "roundtrip.eszip2")
	extractDir := filepath.Join(outDir, "extracted")

	jsFile := filepath.Join(outDir, "hello.js")
	content := []byte("console.log('roundtrip test');\n")
	if err := os.WriteFile(jsFile, content, 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Create
	a, _ := newTestApp()
	if err := a.run([]string{"create", "-o", archivePath, jsFile}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Extract
	a2, _ := newTestApp()
	if err := a2.run([]string{"extract", "-o", extractDir, archivePath}); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// Verify files were extracted
	files := listFilesRecursive(t, extractDir)
	if len(files) == 0 {
		t.Fatal("expected extracted files")
	}

	// Verify at least one file has correct content
	found := false
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if bytes.Equal(data, content) {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find file with original content in extracted output")
	}
}

func TestSpecifierToPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"file:///main.ts", "main.ts"},
		{"file://localhost/main.ts", "localhost/main.ts"},
		{"https://example.com/mod.ts", "example.com/mod.ts"},
		{"http://example.com/mod.ts", "example.com/mod.ts"},
		{"plain/path.ts", "plain/path.ts"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := specifierToPath(tt.input)
			if got != tt.want {
				t.Errorf("specifierToPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
