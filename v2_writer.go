// Copyright 2018-2024 the Deno authors. All rights reserved. MIT license.

package eszip

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// IntoBytes serializes the eszip archive to bytes.
// The context allows cancellation of source slot waits during serialization.
func (e *EszipV2) IntoBytes(ctx context.Context) ([]byte, error) {
	// Snapshot mutable fields under lock
	e.mu.Lock()
	options := e.options
	version := e.version
	npmSnapshot := e.npmSnapshot
	e.mu.Unlock()

	checksum := options.Checksum
	checksumSize := options.GetChecksumSize()

	var result []byte

	// Write magic for the archive's version
	magic := version.ToMagic()
	result = append(result, magic[:]...)

	// Write options header (V2.2+)
	if version.SupportsOptions() {
		optionsHeaderContent := []byte{
			0, byte(checksum), // Checksum type
			1, checksumSize, // Checksum size
		}

		// Write options header length
		optionsHeaderLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(optionsHeaderLenBytes, uint32(len(optionsHeaderContent)))
		result = append(result, optionsHeaderLenBytes...)

		// Write options header content
		result = append(result, optionsHeaderContent...)

		// Write options header hash
		optionsHash := checksum.Hash(optionsHeaderContent)
		result = append(result, optionsHash...)
	}

	// Build modules header, sources, and source maps
	var modulesHeader []byte
	var sources []byte
	var sourceMaps []byte

	keys := e.modules.Keys()
	for _, specifier := range keys {
		mod, ok := e.modules.Get(specifier)
		if !ok {
			continue
		}

		// Write specifier
		if err := appendString(&modulesHeader, specifier); err != nil {
			return nil, err
		}

		switch m := mod.(type) {
		case *ModuleData:
			// Write module entry
			modulesHeader = append(modulesHeader, byte(HeaderFrameModule))

			// Get source bytes
			sourceBytes, err := m.Source.Get(ctx)
			if err != nil {
				return nil, err
			}
			if len(sourceBytes) > math.MaxUint32 {
				return nil, fmt.Errorf("source too large for %s: %d bytes", specifier, len(sourceBytes))
			}
			sourceLen := uint32(len(sourceBytes))

			if sourceLen > 0 {
				if len(sources) > math.MaxUint32 {
					return nil, fmt.Errorf("sources section offset overflow: %d bytes", len(sources))
				}
				sourceOffset := uint32(len(sources))
				sources = append(sources, sourceBytes...)
				sources = append(sources, checksum.Hash(sourceBytes)...)

				modulesHeader = appendU32BE(modulesHeader, sourceOffset)
				modulesHeader = appendU32BE(modulesHeader, sourceLen)
			} else {
				modulesHeader = appendU32BE(modulesHeader, 0)
				modulesHeader = appendU32BE(modulesHeader, 0)
			}

			// Get source map bytes
			sourceMapBytes, err := m.SourceMap.Get(ctx)
			if err != nil {
				return nil, err
			}
			if len(sourceMapBytes) > math.MaxUint32 {
				return nil, fmt.Errorf("source map too large for %s: %d bytes", specifier, len(sourceMapBytes))
			}
			sourceMapLen := uint32(len(sourceMapBytes))

			if sourceMapLen > 0 {
				if len(sourceMaps) > math.MaxUint32 {
					return nil, fmt.Errorf("source maps section offset overflow: %d bytes", len(sourceMaps))
				}
				sourceMapOffset := uint32(len(sourceMaps))
				sourceMaps = append(sourceMaps, sourceMapBytes...)
				sourceMaps = append(sourceMaps, checksum.Hash(sourceMapBytes)...)

				modulesHeader = appendU32BE(modulesHeader, sourceMapOffset)
				modulesHeader = appendU32BE(modulesHeader, sourceMapLen)
			} else {
				modulesHeader = appendU32BE(modulesHeader, 0)
				modulesHeader = appendU32BE(modulesHeader, 0)
			}

			// Write module kind
			modulesHeader = append(modulesHeader, byte(m.Kind))

		case *ModuleRedirect:
			// Write redirect entry
			modulesHeader = append(modulesHeader, byte(HeaderFrameRedirect))
			if err := appendString(&modulesHeader, m.Target); err != nil {
				return nil, err
			}

		case *NpmSpecifierEntry:
			// Write npm specifier entry
			modulesHeader = append(modulesHeader, byte(HeaderFrameNpmSpecifier))
			modulesHeader = appendU32BE(modulesHeader, m.PackageID)
		}
	}

	// Add npm snapshot entries if present (V2.1+)
	var npmBytes []byte
	if npmSnapshot != nil && version.SupportsNpm() {
		// Validate npm snapshot before serialization
		for i, pkg := range npmSnapshot.Packages {
			if pkg == nil || pkg.ID == nil {
				return nil, fmt.Errorf("npm package at index %d has nil ID", i)
			}
			for req, depID := range pkg.Dependencies {
				if depID == nil {
					return nil, fmt.Errorf("npm package %q dependency %q has nil ID", pkg.ID.String(), req)
				}
			}
		}
		for req, id := range npmSnapshot.RootPackages {
			if id == nil {
				return nil, fmt.Errorf("npm root package %q has nil ID", req)
			}
		}

		// Sort packages by ID for determinism
		packages := make([]*NpmPackage, len(npmSnapshot.Packages))
		copy(packages, npmSnapshot.Packages)
		sort.Slice(packages, func(i, j int) bool {
			return packages[i].ID.String() < packages[j].ID.String()
		})

		// Build ID to index map
		idToIndex := make(map[string]uint32)
		for i, pkg := range packages {
			idToIndex[pkg.ID.String()] = uint32(i)
		}

		// Write root packages to modules header
		rootPkgs := make([]struct {
			req string
			id  string
		}, 0, len(npmSnapshot.RootPackages))
		for req, id := range npmSnapshot.RootPackages {
			rootPkgs = append(rootPkgs, struct {
				req string
				id  string
			}{req: req, id: id.String()})
		}
		sort.Slice(rootPkgs, func(i, j int) bool {
			return rootPkgs[i].req < rootPkgs[j].req
		})

		for _, rp := range rootPkgs {
			idx, ok := idToIndex[rp.id]
			if !ok {
				return nil, fmt.Errorf("npm root package %q references unknown package ID %q", rp.req, rp.id)
			}
			if err := appendString(&modulesHeader, rp.req); err != nil {
				return nil, err
			}
			modulesHeader = append(modulesHeader, byte(HeaderFrameNpmSpecifier))
			modulesHeader = appendU32BE(modulesHeader, idx)
		}

		// Write packages to npm bytes
		for _, pkg := range packages {
			if err := appendString(&npmBytes, pkg.ID.String()); err != nil {
				return nil, err
			}

			// Write dependencies count
			npmBytes = appendU32BE(npmBytes, uint32(len(pkg.Dependencies)))

			// Sort dependencies for determinism
			deps := make([]struct {
				req string
				id  string
			}, 0, len(pkg.Dependencies))
			for req, id := range pkg.Dependencies {
				deps = append(deps, struct {
					req string
					id  string
				}{req: req, id: id.String()})
			}
			sort.Slice(deps, func(i, j int) bool {
				return deps[i].req < deps[j].req
			})

			for _, dep := range deps {
				idx, ok := idToIndex[dep.id]
				if !ok {
					return nil, fmt.Errorf("npm package %q dependency %q references unknown package ID %q", pkg.ID.String(), dep.req, dep.id)
				}
				if err := appendString(&npmBytes, dep.req); err != nil {
					return nil, err
				}
				npmBytes = appendU32BE(npmBytes, idx)
			}
		}
	}

	// Write modules header length
	if len(modulesHeader) > math.MaxUint32 {
		return nil, fmt.Errorf("modules header too large: %d bytes", len(modulesHeader))
	}
	modulesHeaderLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(modulesHeaderLenBytes, uint32(len(modulesHeader)))
	result = append(result, modulesHeaderLenBytes...)

	// Write modules header content
	result = append(result, modulesHeader...)

	// Write modules header hash
	modulesHash := checksum.Hash(modulesHeader)
	result = append(result, modulesHash...)

	// Write npm section (V2.1+)
	if version.SupportsNpm() {
		if len(npmBytes) > math.MaxUint32 {
			return nil, fmt.Errorf("npm section too large: %d bytes", len(npmBytes))
		}
		npmLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(npmLenBytes, uint32(len(npmBytes)))
		result = append(result, npmLenBytes...)
		result = append(result, npmBytes...)
		result = append(result, checksum.Hash(npmBytes)...)
	}

	// Write sources section
	if len(sources) > math.MaxUint32 {
		return nil, fmt.Errorf("sources section too large: %d bytes", len(sources))
	}
	sourcesLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(sourcesLenBytes, uint32(len(sources)))
	result = append(result, sourcesLenBytes...)
	result = append(result, sources...)

	// Write source maps section
	if len(sourceMaps) > math.MaxUint32 {
		return nil, fmt.Errorf("source maps section too large: %d bytes", len(sourceMaps))
	}
	sourceMapsLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(sourceMapsLenBytes, uint32(len(sourceMaps)))
	result = append(result, sourceMapsLenBytes...)
	result = append(result, sourceMaps...)

	return result, nil
}

func appendString(buf *[]byte, s string) error {
	if len(s) > math.MaxUint32 {
		return fmt.Errorf("string too large: %d bytes", len(s))
	}
	*buf = binary.BigEndian.AppendUint32(*buf, uint32(len(s)))
	*buf = append(*buf, s...)
	return nil
}

func appendU32BE(buf []byte, v uint32) []byte {
	return binary.BigEndian.AppendUint32(buf, v)
}
