// Copyright 2018-2024 the Deno authors. All rights reserved. MIT license.

// eszip is a CLI tool for working with eszip archives.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	eszip "github.com/JakeChampion/eszip"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("no command specified")
	}

	command := os.Args[1]

	switch command {
	case "view", "v":
		return viewCmd(os.Args[2:])
	case "extract", "x":
		return extractCmd(os.Args[2:])
	case "create", "c":
		return createCmd(os.Args[2:])
	case "info", "i":
		return infoCmd(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Println(`eszip - A tool for working with eszip archives

Usage:
  eszip <command> [options]

Commands:
  view, v       View contents of an eszip archive
  extract, x    Extract files from an eszip archive
  create, c     Create a new eszip archive from files
  info, i       Show information about an eszip archive
  help          Show this help message

Examples:
  eszip view archive.eszip2
  eszip view -s file:///main.ts archive.eszip2
  eszip extract -o ./output archive.eszip2
  cat archive.eszip2 | eszip extract -o ./output
  eszip create -o archive.eszip2 file1.js file2.js
  eszip info archive.eszip2

Run 'eszip <command> -h' for more information on a command.`)
}

// viewCmd handles the 'view' command
func viewCmd(args []string) error {
	fs := flag.NewFlagSet("view", flag.ExitOnError)
	specifier := fs.String("s", "", "Show only this specifier")
	showSourceMap := fs.Bool("m", false, "Show source maps")
	fs.Usage = func() {
		fmt.Println(`Usage: eszip view [options] <archive>

View the contents of an eszip archive.

Options:`)
		fs.PrintDefaults()
	}

	fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("archive path required")
	}

	archivePath := fs.Arg(0)
	ctx := context.Background()

	archive, err := loadArchive(ctx, archivePath)
	if err != nil {
		return err
	}

	specifiers := archive.Specifiers()
	for _, spec := range specifiers {
		if *specifier != "" && spec != *specifier {
			continue
		}

		module := archive.GetModule(spec)
		if module == nil {
			// Might be a redirect-only or npm specifier
			continue
		}

		fmt.Printf("Specifier: %s\n", spec)
		fmt.Printf("Kind: %s\n", module.Kind)
		fmt.Println("---")

		source, err := module.Source(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting source: %v\n", err)
			continue
		}

		if source != nil {
			fmt.Println(string(source))
		} else {
			fmt.Println("(source taken)")
		}

		if *showSourceMap {
			sourceMap, err := module.SourceMap(ctx)
			if err == nil && len(sourceMap) > 0 {
				fmt.Println("--- Source Map ---")
				fmt.Println(string(sourceMap))
			}
		}

		fmt.Println("============")
	}
	return nil
}

// extractCmd handles the 'extract' command
func extractCmd(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	outputDir := fs.String("o", ".", "Output directory")
	fs.Usage = func() {
		fmt.Println(`Usage: eszip extract [options] [<archive>]

Extract files from an eszip archive.
If no archive path is given (or "-" is specified), reads from stdin.

Options:`)
		fs.PrintDefaults()
	}

	fs.Parse(args)

	ctx := context.Background()

	var archive *eszip.EszipUnion
	var err error

	archivePath := fs.Arg(0)
	if archivePath == "" || archivePath == "-" {
		archive, err = loadArchiveFromReader(ctx, os.Stdin)
	} else {
		archive, err = loadArchive(ctx, archivePath)
	}
	if err != nil {
		return err
	}

	specifiers := archive.Specifiers()
	for _, spec := range specifiers {
		module := archive.GetModule(spec)
		if module == nil {
			continue
		}

		// Skip data: URLs
		if strings.HasPrefix(spec, "data:") {
			continue
		}

		source, err := module.Source(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting source for %s: %v\n", spec, err)
			continue
		}

		if source == nil {
			continue
		}

		// Convert specifier to file path
		filePath := specifierToPath(spec)
		fullPath := filepath.Join(*outputDir, filePath)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
			continue
		}

		// Write file
		if err := os.WriteFile(fullPath, source, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			continue
		}

		fmt.Printf("Extracted: %s\n", fullPath)

		// Also extract source map if available
		sourceMap, err := module.SourceMap(ctx)
		if err == nil && len(sourceMap) > 0 {
			mapPath := fullPath + ".map"
			if err := os.WriteFile(mapPath, sourceMap, 0644); err == nil {
				fmt.Printf("Extracted: %s\n", mapPath)
			}
		}
	}
	return nil
}

// createCmd handles the 'create' command
func createCmd(args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	outputPath := fs.String("o", "output.eszip2", "Output file path")
	checksum := fs.String("checksum", "sha256", "Checksum algorithm (none, sha256, xxhash3)")
	fs.Usage = func() {
		fmt.Println(`Usage: eszip create [options] <files...>

Create a new eszip archive from files.

Options:`)
		fs.PrintDefaults()
		fmt.Println(`
Examples:
  eszip create -o app.eszip2 main.js utils.js
  eszip create -checksum none -o app.eszip2 *.js`)
	}

	fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("at least one file required")
	}

	archive := eszip.NewV2()

	// Set checksum
	switch *checksum {
	case "none":
		archive.SetChecksum(eszip.ChecksumNone)
	case "sha256":
		archive.SetChecksum(eszip.ChecksumSha256)
	case "xxhash3":
		archive.SetChecksum(eszip.ChecksumXxh3)
	default:
		return fmt.Errorf("unknown checksum: %s", *checksum)
	}

	// Add files
	for _, filePath := range fs.Args() {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("resolving path %s: %w", filePath, err)
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", filePath, err)
		}

		// Determine module kind
		kind := eszip.ModuleKindJavaScript
		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".json":
			kind = eszip.ModuleKindJson
		case ".wasm":
			kind = eszip.ModuleKindWasm
		}

		specifier := "file://" + absPath
		archive.AddModule(specifier, kind, content, nil)
		fmt.Printf("Added: %s\n", specifier)
	}

	// Serialize
	data, err := archive.IntoBytes()
	if err != nil {
		return fmt.Errorf("serializing archive: %w", err)
	}

	// Write output
	if err := os.WriteFile(*outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("Created: %s (%d bytes)\n", *outputPath, len(data))
	return nil
}

// infoCmd handles the 'info' command
func infoCmd(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Println(`Usage: eszip info <archive>

Show information about an eszip archive.`)
	}

	fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("archive path required")
	}

	archivePath := fs.Arg(0)
	ctx := context.Background()

	// Get file size
	stat, err := os.Stat(archivePath)
	if err != nil {
		return err
	}

	archive, err := loadArchive(ctx, archivePath)
	if err != nil {
		return err
	}

	specifiers := archive.Specifiers()

	fmt.Printf("File: %s\n", archivePath)
	fmt.Printf("Size: %d bytes\n", stat.Size())

	if archive.IsV1() {
		fmt.Println("Format: V1 (JSON)")
	} else {
		fmt.Println("Format: V2 (binary)")
	}

	fmt.Printf("Modules: %d\n", len(specifiers))

	// Count by kind
	kindCounts := make(map[eszip.ModuleKind]int)
	redirectCount := 0
	totalSourceSize := 0

	for _, spec := range specifiers {
		module := archive.GetModule(spec)
		if module == nil {
			redirectCount++
			continue
		}
		kindCounts[module.Kind]++

		source, _ := module.Source(ctx)
		totalSourceSize += len(source)
	}

	fmt.Println("\nModule types:")
	for kind, count := range kindCounts {
		fmt.Printf("  %s: %d\n", kind, count)
	}
	if redirectCount > 0 {
		fmt.Printf("  redirects: %d\n", redirectCount)
	}

	fmt.Printf("\nTotal source size: %d bytes\n", totalSourceSize)

	// Check for npm snapshot
	if archive.IsV2() {
		snapshot := archive.V2().TakeNpmSnapshot()
		if snapshot != nil {
			fmt.Printf("\nNPM packages: %d\n", len(snapshot.Packages))
			fmt.Printf("NPM root packages: %d\n", len(snapshot.RootPackages))
		}
	}
	return nil
}

func loadArchive(ctx context.Context, path string) (*eszip.EszipUnion, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return loadArchiveFromReader(ctx, f)
}

func loadArchiveFromReader(ctx context.Context, r io.Reader) (*eszip.EszipUnion, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	return eszip.ParseBytes(ctx, data)
}

func specifierToPath(specifier string) string {
	path := specifier
	for _, prefix := range []string{"file:///", "file://", "https://", "http://"} {
		if after, found := strings.CutPrefix(path, prefix); found {
			path = after
			break
		}
	}
	path = strings.TrimPrefix(path, "/")
	return path
}
