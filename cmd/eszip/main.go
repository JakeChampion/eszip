// Copyright 2018-2024 the Deno authors. All rights reserved. MIT license.

// eszip is a CLI tool for working with eszip archives.
package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/JakeChampion/eszip"
	"github.com/spf13/cobra"
)

type app struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

func main() {
	a := &app{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin}
	if err := a.rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (a *app) rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eszip",
		Short: "A tool for working with eszip archives",
		Long: `eszip - A tool for working with eszip archives

Examples:
  eszip view archive.eszip2
  eszip view -s file:///main.ts archive.eszip2
  eszip extract -o ./output archive.eszip2
  cat archive.eszip2 | eszip extract -o ./output
  eszip create -o archive.eszip2 file1.js file2.js
  eszip info archive.eszip2`,
		SilenceErrors: true,
		// Show usage for flag/arg errors but not for runtime errors.
		// PersistentPreRun fires after flag parsing succeeds, so any
		// error returned by RunE will not print usage.
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			cmd.SilenceUsage = true
		},
	}

	cmd.SetOut(a.stdout)
	cmd.SetErr(a.stderr)
	cmd.SetIn(a.stdin)

	cmd.AddCommand(
		a.viewCmd(),
		a.extractCmd(),
		a.createCmd(),
		a.infoCmd(),
	)

	return cmd
}

func (a *app) viewCmd() *cobra.Command {
	var specifier string
	var showSourceMap bool
	var listOnly bool

	cmd := &cobra.Command{
		Use:     "view <archive>",
		Aliases: []string{"v"},
		Short:   "View contents of an eszip archive",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()

			archive, err := loadArchive(ctx, args[0])
			if err != nil {
				return err
			}

			for _, spec := range archive.Specifiers() {
				if specifier != "" && spec != specifier {
					continue
				}

				module := archive.GetModule(spec)
				if module == nil {
					continue
				}

				if listOnly {
					fmt.Fprintln(a.stdout, spec)
					continue
				}

				fmt.Fprintf(a.stdout, "Specifier: %s\n", spec)
				fmt.Fprintf(a.stdout, "Kind: %s\n", module.Kind)
				fmt.Fprintln(a.stdout, "---")

				source, err := module.Source(ctx)
				if err != nil {
					fmt.Fprintf(a.stderr, "Error getting source: %v\n", err)
					continue
				}

				if source != nil {
					fmt.Fprintln(a.stdout, string(source))
				} else {
					fmt.Fprintln(a.stdout, "(source taken)")
				}

				if showSourceMap {
					sourceMap, err := module.SourceMap(ctx)
					if err == nil && len(sourceMap) > 0 {
						fmt.Fprintln(a.stdout, "--- Source Map ---")
						fmt.Fprintln(a.stdout, string(sourceMap))
					}
				}

				fmt.Fprintln(a.stdout, "============")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&specifier, "specifier", "s", "", "Show only this specifier")
	cmd.Flags().BoolVarP(&showSourceMap, "source-map", "m", false, "Show source maps")
	cmd.Flags().BoolVarP(&listOnly, "list", "l", false, "List specifiers only")

	return cmd
}

func (a *app) extractCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:     "extract [<archive>]",
		Aliases: []string{"x"},
		Short:   "Extract files from an eszip archive",
		Long: `Extract files from an eszip archive.
If no archive path is given (or "-" is specified), reads from stdin.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()

			var archive *eszip.EszipUnion
			var err error

			if len(args) == 0 || args[0] == "-" {
				archive, err = loadArchiveFromReader(ctx, a.stdin)
			} else {
				archive, err = loadArchive(ctx, args[0])
			}
			if err != nil {
				return err
			}

			var errCount int
			for _, spec := range archive.Specifiers() {
				module := archive.GetModule(spec)
				if module == nil {
					continue
				}

				if strings.HasPrefix(spec, "data:") {
					continue
				}

				source, err := module.Source(ctx)
				if err != nil {
					fmt.Fprintf(a.stderr, "Error getting source for %s: %v\n", spec, err)
					errCount++
					continue
				}

				if source == nil {
					continue
				}

				filePath := specifierToPath(spec)
				fullPath := filepath.Join(outputDir, filePath)

				// Guard against path traversal: ensure the resolved
				// path stays inside the output directory.
				absOut, err := filepath.Abs(outputDir)
				if err != nil {
					fmt.Fprintf(a.stderr, "Error resolving output dir: %v\n", err)
					errCount++
					continue
				}
				absFull, err := filepath.Abs(fullPath)
				if err != nil {
					fmt.Fprintf(a.stderr, "Error resolving path: %v\n", err)
					errCount++
					continue
				}
				if !strings.HasPrefix(absFull, absOut+string(filepath.Separator)) && absFull != absOut {
					fmt.Fprintf(a.stderr, "Skipping %s: path escapes output directory\n", spec)
					errCount++
					continue
				}

				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					fmt.Fprintf(a.stderr, "Error creating directory: %v\n", err)
					errCount++
					continue
				}

				if err := os.WriteFile(fullPath, source, 0644); err != nil {
					fmt.Fprintf(a.stderr, "Error writing file: %v\n", err)
					errCount++
					continue
				}

				fmt.Fprintf(a.stdout, "Extracted: %s\n", fullPath)

				sourceMap, err := module.SourceMap(ctx)
				if err == nil && len(sourceMap) > 0 {
					mapPath := fullPath + ".map"
					absMap, err := filepath.Abs(mapPath)
					if err != nil {
						fmt.Fprintf(a.stderr, "Error resolving source map path: %v\n", err)
						errCount++
					} else if !strings.HasPrefix(absMap, absOut+string(filepath.Separator)) && absMap != absOut {
						fmt.Fprintf(a.stderr, "Skipping source map for %s: path escapes output directory\n", spec)
						errCount++
					} else if err := os.WriteFile(mapPath, sourceMap, 0644); err != nil {
						fmt.Fprintf(a.stderr, "Error writing source map: %v\n", err)
						errCount++
					} else {
						fmt.Fprintf(a.stdout, "Extracted: %s\n", mapPath)
					}
				}
			}
			if errCount > 0 {
				return fmt.Errorf("extraction completed with %d error(s)", errCount)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", ".", "Output directory")

	return cmd
}

func (a *app) createCmd() *cobra.Command {
	var outputPath string
	var checksum string

	cmd := &cobra.Command{
		Use:     "create <files...>",
		Aliases: []string{"c"},
		Short:   "Create a new eszip archive from files",
		Example: `  eszip create -o app.eszip2 main.js utils.js
  eszip create --checksum none -o app.eszip2 *.js`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			archive := eszip.NewV2()

			switch checksum {
			case "none":
				archive.SetChecksum(eszip.ChecksumNone)
			case "sha256":
				archive.SetChecksum(eszip.ChecksumSha256)
			case "xxhash3":
				archive.SetChecksum(eszip.ChecksumXxh3)
			default:
				return fmt.Errorf("unknown checksum: %s", checksum)
			}

			for _, filePath := range args {
				absPath, err := filepath.Abs(filePath)
				if err != nil {
					return fmt.Errorf("resolving path %s: %w", filePath, err)
				}

				content, err := os.ReadFile(absPath)
				if err != nil {
					return fmt.Errorf("reading file %s: %w", filePath, err)
				}

				kind := eszip.ModuleKindJavaScript
				ext := strings.ToLower(filepath.Ext(filePath))
				switch ext {
				case ".json":
					kind = eszip.ModuleKindJson
				case ".wasm":
					kind = eszip.ModuleKindWasm
				}

				specifier := (&url.URL{Scheme: "file", Path: absPath}).String()
				archive.AddModule(specifier, kind, content, nil)
				fmt.Fprintf(a.stdout, "Added: %s\n", specifier)
			}

			data, err := archive.IntoBytes(ctx)
			if err != nil {
				return fmt.Errorf("serializing archive: %w", err)
			}

			if err := os.WriteFile(outputPath, data, 0644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}

			fmt.Fprintf(a.stdout, "Created: %s (%d bytes)\n", outputPath, len(data))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "output.eszip2", "Output file path")
	cmd.Flags().StringVar(&checksum, "checksum", "sha256", "Checksum algorithm (none, sha256, xxhash3)")

	return cmd
}

func (a *app) infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "info <archive>",
		Aliases: []string{"i"},
		Short:   "Show information about an eszip archive",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			archivePath := args[0]
			ctx := context.Background()

			stat, err := os.Stat(archivePath)
			if err != nil {
				return err
			}

			archive, err := loadArchive(ctx, archivePath)
			if err != nil {
				return err
			}

			specifiers := archive.Specifiers()

			fmt.Fprintf(a.stdout, "File: %s\n", archivePath)
			fmt.Fprintf(a.stdout, "Size: %d bytes\n", stat.Size())

			if archive.IsV1() {
				fmt.Fprintln(a.stdout, "Format: V1 (JSON)")
			} else {
				fmt.Fprintln(a.stdout, "Format: V2 (binary)")
			}

			fmt.Fprintf(a.stdout, "Modules: %d\n", len(specifiers))

			kindCounts := make(map[eszip.ModuleKind]int)
			otherCount := 0
			totalSourceSize := 0

			for _, spec := range specifiers {
				module := archive.GetModule(spec)
				if module == nil {
					otherCount++
					continue
				}
				kindCounts[module.Kind]++

				source, err := module.Source(ctx)
				if err != nil {
					fmt.Fprintf(a.stderr, "Error getting source for %s: %v\n", spec, err)
					continue
				}
				totalSourceSize += len(source)
			}

			fmt.Fprintln(a.stdout, "\nModule types:")
			for kind, count := range kindCounts {
				fmt.Fprintf(a.stdout, "  %s: %d\n", kind, count)
			}
			if otherCount > 0 {
				fmt.Fprintf(a.stdout, "  other (redirects/npm): %d\n", otherCount)
			}

			fmt.Fprintf(a.stdout, "\nTotal source size: %d bytes\n", totalSourceSize)

			if v2, ok := archive.V2(); ok {
				snapshot := v2.NpmSnapshot()
				if snapshot != nil {
					fmt.Fprintf(a.stdout, "\nNPM packages: %d\n", len(snapshot.Packages))
					fmt.Fprintf(a.stdout, "NPM root packages: %d\n", len(snapshot.RootPackages))
				}
			}
			return nil
		},
	}
}

func loadArchive(ctx context.Context, path string) (_ *eszip.EszipUnion, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()
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
