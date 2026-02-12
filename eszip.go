// Copyright 2018-2024 the Deno authors. All rights reserved. MIT license.

// Package eszip provides functionality for reading and writing eszip archives.
// Eszip is a binary serialization format for ECMAScript module graphs, used by Deno.
package eszip

import (
	"bufio"
	"bytes"
	"context"
	"io"
)

// EszipUnion wraps either V1 or V2 eszip
type EszipUnion struct {
	v1 *EszipV1
	v2 *EszipV2
}

// IsV1 returns true if this is a V1 archive
func (e *EszipUnion) IsV1() bool {
	return e.v1 != nil
}

// IsV2 returns true if this is a V2 archive
func (e *EszipUnion) IsV2() bool {
	return e.v2 != nil
}

// V1 returns the V1 archive, or nil and false if not V1
func (e *EszipUnion) V1() (*EszipV1, bool) {
	return e.v1, e.v1 != nil
}

// V2 returns the V2 archive, or nil and false if not V2
func (e *EszipUnion) V2() (*EszipV2, bool) {
	return e.v2, e.v2 != nil
}

// GetModule returns the module for the given specifier
func (e *EszipUnion) GetModule(specifier string) *Module {
	if e.v1 != nil {
		return e.v1.GetModule(specifier)
	}
	if e.v2 != nil {
		return e.v2.GetModule(specifier)
	}
	return nil
}

// GetImportMap returns the import map module for the given specifier
func (e *EszipUnion) GetImportMap(specifier string) *Module {
	if e.v1 != nil {
		return e.v1.GetImportMap(specifier)
	}
	if e.v2 != nil {
		return e.v2.GetImportMap(specifier)
	}
	return nil
}

// Specifiers returns all module specifiers
func (e *EszipUnion) Specifiers() []string {
	if e.v1 != nil {
		return e.v1.Specifiers()
	}
	if e.v2 != nil {
		return e.v2.Specifiers()
	}
	return nil
}

// NpmSnapshot returns the NPM snapshot without removing it
func (e *EszipUnion) NpmSnapshot() *NpmResolutionSnapshot {
	if e.v2 != nil {
		return e.v2.NpmSnapshot()
	}
	return nil
}

// TakeNpmSnapshot removes and returns the NPM snapshot
func (e *EszipUnion) TakeNpmSnapshot() *NpmResolutionSnapshot {
	if e.v2 != nil {
		return e.v2.TakeNpmSnapshot()
	}
	return nil
}

// Parse parses an eszip archive from the given reader.
// Returns the eszip and a function to complete parsing of source data (for streaming).
// The completion function must be called to fully load sources.
func Parse(ctx context.Context, r io.Reader) (*EszipUnion, func(context.Context) error, error) {
	br := bufio.NewReader(r)

	// Read magic bytes
	magic := make([]byte, 8)
	if _, err := io.ReadFull(br, magic); err != nil {
		return nil, nil, errIO(err)
	}

	// Check if it's V2
	if version, ok := VersionFromMagic(magic); ok {
		eszip, complete, err := parseV2WithVersion(ctx, version, br)
		if err != nil {
			return nil, nil, err
		}
		return &EszipUnion{v2: eszip}, complete, nil
	}

	// Otherwise, treat as V1 JSON - read the rest
	var allData []byte
	allData = append(allData, magic...)
	remaining, err := io.ReadAll(br)
	if err != nil {
		return nil, nil, errIO(err)
	}
	allData = append(allData, remaining...)

	eszip, err := ParseV1(allData)
	if err != nil {
		return nil, nil, err
	}

	// V1 has no streaming, completion is a no-op
	complete := func(ctx context.Context) error {
		return nil
	}

	return &EszipUnion{v1: eszip}, complete, nil
}

// ParseSync parses an eszip archive completely (blocking)
func ParseSync(ctx context.Context, r io.Reader) (*EszipUnion, error) {
	eszip, complete, err := Parse(ctx, r)
	if err != nil {
		return nil, err
	}

	if err := complete(ctx); err != nil {
		return nil, err
	}

	return eszip, nil
}

// ParseBytes parses an eszip from a byte slice
func ParseBytes(ctx context.Context, data []byte) (*EszipUnion, error) {
	return ParseSync(ctx, bytes.NewReader(data))
}

// NewV2 creates a new empty V2 eszip archive
func NewV2() *EszipV2 {
	return &EszipV2{
		modules: NewModuleMap(),
		options: DefaultOptionsForVersion(LatestVersion),
		version: LatestVersion,
	}
}
