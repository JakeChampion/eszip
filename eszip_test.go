// Copyright 2018-2024 the Deno authors. All rights reserved. MIT license.

package eszip

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParseV1(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	if !eszip.IsV1() {
		t.Fatal("expected V1 eszip")
	}

	specifier := "https://gist.githubusercontent.com/lucacasonato/f3e21405322259ca4ed155722390fda2/raw/e25acb49b681e8e1da5a2a33744b7a36d538712d/hello.js"
	module := eszip.GetModule(specifier)
	if module == nil {
		t.Fatalf("expected to find module: %s", specifier)
		return
	}

	if module.Specifier != specifier {
		t.Errorf("expected specifier %s, got %s", specifier, module.Specifier)
	}

	if module.Kind != ModuleKindJavaScript {
		t.Errorf("expected JavaScript module, got %v", module.Kind)
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}

	if len(source) == 0 {
		t.Error("expected non-empty source")
	}

	// Verify source contains expected content
	if !bytes.Contains(source, []byte("Hello World")) {
		t.Error("source should contain 'Hello World'")
	}
}

func TestParseV2(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	if !eszip.IsV2() {
		t.Fatal("expected V2 eszip")
	}

	module := eszip.GetModule("file:///main.ts")
	if module == nil {
		t.Fatal("expected to find module: file:///main.ts")
		return
	}

	if module.Kind != ModuleKindJavaScript {
		t.Errorf("expected JavaScript module, got %v", module.Kind)
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}

	expectedSource := `export * as a from "./a.ts";
`
	if string(source) != expectedSource {
		t.Errorf("expected source %q, got %q", expectedSource, string(source))
	}

	// Test source map
	sourceMap, err := module.SourceMap(ctx)
	if err != nil {
		t.Fatalf("failed to get source map: %v", err)
	}

	if len(sourceMap) == 0 {
		t.Error("expected non-empty source map")
	}
}

func TestV2Redirect(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	// file:///a.ts is a redirect to file:///b.ts
	moduleA := eszip.GetModule("file:///a.ts")
	if moduleA == nil {
		t.Fatal("expected to find module: file:///a.ts")
	}

	moduleB := eszip.GetModule("file:///b.ts")
	if moduleB == nil {
		t.Fatal("expected to find module: file:///b.ts")
	}

	sourceA, _ := moduleA.Source(ctx)
	sourceB, _ := moduleB.Source(ctx)

	// Both should have the same source since a.ts redirects to b.ts
	if !bytes.Equal(sourceA, sourceB) {
		t.Errorf("expected same source for redirect, got %q and %q", string(sourceA), string(sourceB))
	}
}

func TestTakeSource(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	specifier := "https://gist.githubusercontent.com/lucacasonato/f3e21405322259ca4ed155722390fda2/raw/e25acb49b681e8e1da5a2a33744b7a36d538712d/hello.js"
	module := eszip.GetModule(specifier)
	if module == nil {
		t.Fatalf("expected to find module: %s", specifier)
	}

	// Take the source
	source, err := module.TakeSource(ctx)
	if err != nil {
		t.Fatalf("failed to take source: %v", err)
	}

	if len(source) == 0 {
		t.Error("expected non-empty source")
	}

	// Module should no longer be available in V1
	module2 := eszip.GetModule(specifier)
	if module2 != nil {
		t.Error("expected module to be removed after take (V1 behavior)")
	}
}

func TestV2TakeSource(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	module := eszip.GetModule("file:///main.ts")
	if module == nil {
		t.Fatal("expected to find module")
	}

	// Take the source
	source, err := module.TakeSource(ctx)
	if err != nil {
		t.Fatalf("failed to take source: %v", err)
	}

	if len(source) == 0 {
		t.Error("expected non-empty source")
	}

	// Module should still be available but source should be nil
	module2 := eszip.GetModule("file:///main.ts")
	if module2 == nil {
		t.Fatal("expected module to still exist in V2")
	}

	source2, err := module2.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}
	if source2 != nil {
		t.Error("expected source to be nil after take")
	}

	// Source map should still be available
	sourceMap, err := module2.SourceMap(ctx)
	if err != nil {
		t.Fatalf("failed to get source map: %v", err)
	}
	if len(sourceMap) == 0 {
		t.Error("expected source map to still be available")
	}
}

func TestV2Specifiers(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	specs := eszip.Specifiers()
	if len(specs) == 0 {
		t.Error("expected at least one specifier")
	}

	// Should contain main.ts, b.ts, and a.ts
	expected := map[string]bool{
		"file:///main.ts": true,
		"file:///b.ts":    true,
		"file:///a.ts":    true,
	}

	for _, spec := range specs {
		delete(expected, spec)
	}

	if len(expected) > 0 {
		t.Errorf("missing specifiers: %v", expected)
	}
}

func TestNewV2AndWrite(t *testing.T) {
	ctx := context.Background()

	// Create a new V2 eszip
	eszip := NewV2()

	// Add a module
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("console.log('hello');"), []byte("{}"))

	// Add a redirect
	eszip.AddRedirect("file:///alias.js", "file:///test.js")

	// Serialize
	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize eszip: %v", err)
	}

	// Parse it back
	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse serialized eszip: %v", err)
	}

	if !parsed.IsV2() {
		t.Fatal("expected V2 eszip")
	}

	// Verify the module
	module := parsed.GetModule("file:///test.js")
	if module == nil {
		t.Fatal("expected to find module")
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}

	if string(source) != "console.log('hello');" {
		t.Errorf("expected source %q, got %q", "console.log('hello');", string(source))
	}

	// Verify the redirect
	aliasModule := parsed.GetModule("file:///alias.js")
	if aliasModule == nil {
		t.Fatal("expected to find alias module")
	}

	aliasSource, err := aliasModule.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get alias source: %v", err)
	}

	if string(aliasSource) != "console.log('hello');" {
		t.Errorf("expected alias source %q, got %q", "console.log('hello');", string(aliasSource))
	}
}

func TestChecksumTypes(t *testing.T) {
	testCases := []struct {
		name     string
		checksum ChecksumType
	}{
		{"NoChecksum", ChecksumNone},
		{"Sha256", ChecksumSha256},
		{"XxHash3", ChecksumXxh3},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eszip := NewV2()
			eszip.SetChecksum(tc.checksum)
			eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("test"), nil)

			data, err := eszip.IntoBytes()
			if err != nil {
				t.Fatalf("failed to serialize: %v", err)
			}

			parsed, err := ParseBytes(ctx, data)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			module := parsed.GetModule("file:///test.js")
			if module == nil {
				t.Fatal("expected to find module")
			}

			source, err := module.Source(ctx)
			if err != nil {
				t.Fatalf("failed to get source: %v", err)
			}

			if string(source) != "test" {
				t.Errorf("expected source 'test', got %q", string(source))
			}
		})
	}
}

func TestModuleKinds(t *testing.T) {
	testCases := []struct {
		kind ModuleKind
		name string
	}{
		{ModuleKindJavaScript, "javascript"},
		{ModuleKindJson, "json"},
		{ModuleKindJsonc, "jsonc"},
		{ModuleKindOpaqueData, "opaque_data"},
		{ModuleKindWasm, "wasm"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind.String() != tc.name {
				t.Errorf("expected %s, got %s", tc.name, tc.kind.String())
			}
		})
	}
}

func TestV1Iterator(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	eszip, err := ParseV1(data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	modules := eszip.Iterate()
	if len(modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(modules))
	}
}

func TestV2Iterator(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse eszip: %v", err)
	}

	v2, ok := parsed.V2()
	if !ok {
		t.Fatal("expected V2 eszip")
	}
	modules := v2.Iterate()
	// Should have 3 modules but only 2 are actual modules (one is redirect)
	if len(modules) < 2 {
		t.Errorf("expected at least 2 modules, got %d", len(modules))
	}
}

// --- V2 parsing error path tests ---

func TestParseEmptyData(t *testing.T) {
	ctx := context.Background()
	_, err := ParseBytes(ctx, []byte{})
	if err == nil {
		t.Fatal("expected error parsing empty data")
	}
}

func TestParseTruncatedMagic(t *testing.T) {
	ctx := context.Background()
	_, err := ParseBytes(ctx, []byte("ESZI"))
	if err == nil {
		t.Fatal("expected error parsing truncated magic")
	}
}

func TestParseInvalidV1Json(t *testing.T) {
	ctx := context.Background()
	_, err := ParseBytes(ctx, []byte("not json at all!!!"))
	if err == nil {
		t.Fatal("expected error parsing invalid JSON")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Type != ErrInvalidV1Json {
		t.Errorf("expected ErrInvalidV1Json, got %v", pe.Type)
	}
}

func TestParseV1WrongVersion(t *testing.T) {
	_, err := ParseV1([]byte(`{"version":99,"modules":{}}`))
	if err == nil {
		t.Fatal("expected error for wrong version")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if pe.Type != ErrInvalidV1Version {
		t.Errorf("expected ErrInvalidV1Version, got %v", pe.Type)
	}
}

func TestParseV2TruncatedAfterMagic(t *testing.T) {
	ctx := context.Background()
	// Valid magic but nothing after it
	_, err := ParseBytes(ctx, MagicV2_3[:])
	if err == nil {
		t.Fatal("expected error for truncated V2")
	}
}

func TestParseV2CorruptHeaderHash(t *testing.T) {
	ctx := context.Background()

	// Create a valid archive, then corrupt the header hash
	eszip := NewV2()
	eszip.SetChecksum(ChecksumSha256)
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("test"), nil)

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	// The options header hash starts after magic(8) + options_len(4) + options_content(4)
	// = offset 16, and is 32 bytes. Corrupt it.
	if len(data) > 48 {
		data[16] ^= 0xff
	}

	_, err = ParseBytes(ctx, data)
	if err == nil {
		t.Fatal("expected error for corrupt header hash")
	}
}

func TestParseV2CorruptSourceData(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.SetChecksum(ChecksumSha256)
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("hello world"), nil)

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	// Find the sources section. After magic(8) + options header + modules header + npm section,
	// there's a 4-byte sources length. Find it by looking for the source content.
	sourceContent := []byte("hello world")
	idx := bytes.Index(data, sourceContent)
	if idx < 0 {
		t.Fatal("could not find source content in serialized data")
	}

	// Corrupt the source content itself - the hash check should then fail
	data[idx] ^= 0xff

	_, err = ParseBytes(ctx, data)
	if err == nil {
		t.Fatal("expected error for corrupt source data")
	}
}

func TestParseErrorFormat(t *testing.T) {
	pe := &ParseError{Type: ErrInvalidV2, Message: "test error"}
	if !strings.Contains(pe.Error(), "test error") {
		t.Errorf("expected error message to contain 'test error', got %q", pe.Error())
	}

	pe2 := &ParseError{Type: ErrInvalidV2SourceOffset, Message: "offset error", Offset: 42}
	got := pe2.Error()
	if !strings.Contains(got, "offset 42") {
		t.Errorf("expected error to contain 'offset 42', got %q", got)
	}
}

// --- V1 roundtrip and source map tests ---

func TestV1IntoBytes(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	eszip, err := ParseV1(data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Roundtrip
	serialized, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	// Parse again
	eszip2, err := ParseV1(serialized)
	if err != nil {
		t.Fatalf("failed to re-parse: %v", err)
	}

	specs := eszip2.Specifiers()
	if len(specs) == 0 {
		t.Fatal("expected at least one specifier after roundtrip")
	}
}

func TestV1SourceMap(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	specifier := "https://gist.githubusercontent.com/lucacasonato/f3e21405322259ca4ed155722390fda2/raw/e25acb49b681e8e1da5a2a33744b7a36d538712d/hello.js"
	module := eszip.GetModule(specifier)
	if module == nil {
		t.Fatal("expected to find module")
	}

	// V1 does not support source maps
	sm, err := module.SourceMap(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm != nil {
		t.Error("expected nil source map for V1")
	}

	// TakeSourceMap should also return nil
	sm2, err := module.TakeSourceMap(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm2 != nil {
		t.Error("expected nil TakeSourceMap for V1")
	}
}

func TestV1GetImportMap(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// V1 never has import maps
	im := eszip.GetImportMap("anything")
	if im != nil {
		t.Error("expected nil import map for V1")
	}
}

func TestV1NonexistentModule(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	module := eszip.GetModule("file:///nonexistent.js")
	if module != nil {
		t.Error("expected nil for nonexistent module")
	}

	// Also test via EszipUnion
	_ = eszip.Specifiers()
}

func TestEszipUnionV1(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if !eszip.IsV1() {
		t.Fatal("expected V1")
	}
	if eszip.IsV2() {
		t.Fatal("expected not V2")
	}

	v1, ok := eszip.V1()
	if !ok || v1 == nil {
		t.Fatal("expected V1() to return non-nil")
	}

	v2, ok := eszip.V2()
	if ok || v2 != nil {
		t.Fatal("expected V2() to return nil for V1 archive")
	}

	// TakeNpmSnapshot on V1 should return nil
	snapshot := eszip.TakeNpmSnapshot()
	if snapshot != nil {
		t.Error("expected nil npm snapshot for V1")
	}
}

// --- ModuleMap tests ---

func TestModuleMapInsertFront(t *testing.T) {
	m := NewModuleMap()
	m.Insert("a", &ModuleData{Kind: ModuleKindJavaScript})
	m.Insert("b", &ModuleData{Kind: ModuleKindJson})
	m.InsertFront("c", &ModuleData{Kind: ModuleKindWasm})

	keys := m.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "c" {
		t.Errorf("expected 'c' at front, got %q", keys[0])
	}
}

func TestModuleMapInsertFrontExisting(t *testing.T) {
	m := NewModuleMap()
	m.Insert("a", &ModuleData{Kind: ModuleKindJavaScript})
	m.Insert("b", &ModuleData{Kind: ModuleKindJson})
	m.Insert("c", &ModuleData{Kind: ModuleKindWasm})

	// InsertFront with existing key should move it to front
	m.InsertFront("b", &ModuleData{Kind: ModuleKindOpaqueData})

	keys := m.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "b" {
		t.Errorf("expected 'b' at front, got %q", keys[0])
	}

	// Verify the data was updated
	mod, ok := m.Get("b")
	if !ok {
		t.Fatal("expected to find 'b'")
	}
	data := mod.(*ModuleData)
	if data.Kind != ModuleKindOpaqueData {
		t.Errorf("expected OpaqueData kind, got %v", data.Kind)
	}
}

func TestModuleMapRemove(t *testing.T) {
	m := NewModuleMap()
	m.Insert("a", &ModuleData{Kind: ModuleKindJavaScript})
	m.Insert("b", &ModuleData{Kind: ModuleKindJson})
	m.Insert("c", &ModuleData{Kind: ModuleKindWasm})

	mod, ok := m.Remove("b")
	if !ok {
		t.Fatal("expected to remove 'b'")
	}
	if mod == nil {
		t.Fatal("expected non-nil module")
	}

	keys := m.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Verify 'b' is gone
	_, ok = m.Get("b")
	if ok {
		t.Error("expected 'b' to be removed")
	}

	// Remove nonexistent
	_, ok = m.Remove("nonexistent")
	if ok {
		t.Error("expected false for nonexistent key")
	}
}

func TestModuleMapLen(t *testing.T) {
	m := NewModuleMap()
	if m.Len() != 0 {
		t.Errorf("expected 0, got %d", m.Len())
	}

	m.Insert("a", &ModuleData{Kind: ModuleKindJavaScript})
	m.Insert("b", &ModuleData{Kind: ModuleKindJson})
	if m.Len() != 2 {
		t.Errorf("expected 2, got %d", m.Len())
	}

	// Insert same key again shouldn't change count
	m.Insert("a", &ModuleData{Kind: ModuleKindWasm})
	if m.Len() != 2 {
		t.Errorf("expected 2 after re-insert, got %d", m.Len())
	}
}

// --- Module Take/Get and context cancellation ---

func TestSourceSlotContextCancellation(t *testing.T) {
	slot := NewPendingSourceSlot(0, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := slot.Get(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSourceSlotTakeContextCancellation(t *testing.T) {
	slot := NewPendingSourceSlot(0, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := slot.Take(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSourceSlotGetAfterTake(t *testing.T) {
	slot := NewReadySourceSlot([]byte("hello"))

	ctx := context.Background()

	// Take the data
	data, err := slot.Take(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}

	if slot.State() != SourceSlotTaken {
		t.Errorf("expected Taken state, got %v", slot.State())
	}

	// Get after take should return nil
	data2, err := slot.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data2 != nil {
		t.Error("expected nil after take")
	}

	// Take again should return nil
	data3, err := slot.Take(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data3 != nil {
		t.Error("expected nil on second take")
	}
}

func TestSourceSlotSetReadyThenGet(t *testing.T) {
	slot := NewPendingSourceSlot(0, 5)
	ctx := context.Background()

	// Set ready in a goroutine
	go func() {
		slot.SetReady([]byte("world"))
	}()

	data, err := slot.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("expected 'world', got %q", string(data))
	}
}

func TestV2TakeSourceMap(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("code"), []byte("sourcemap"))

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	module := parsed.GetModule("file:///test.js")
	if module == nil {
		t.Fatal("expected to find module")
	}

	sm, err := module.TakeSourceMap(ctx)
	if err != nil {
		t.Fatalf("failed to take source map: %v", err)
	}
	if string(sm) != "sourcemap" {
		t.Errorf("expected 'sourcemap', got %q", string(sm))
	}

	// Second take returns nil
	sm2, err := module.SourceMap(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm2 != nil {
		t.Error("expected nil source map after take")
	}
}

// --- Import map and opaque data tests ---

func TestAddImportMap(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddModule("file:///main.js", ModuleKindJavaScript, []byte("import 'foo'"), nil)
	eszip.AddImportMap(ModuleKindJson, "file:///import_map.json", []byte(`{"imports":{"foo":"./bar.js"}}`))

	// Import map should be at the front
	specs := eszip.Specifiers()
	if len(specs) != 2 {
		t.Fatalf("expected 2 specifiers, got %d", len(specs))
	}
	if specs[0] != "file:///import_map.json" {
		t.Errorf("expected import map at front, got %q", specs[0])
	}

	// GetImportMap should work for jsonc kinds too, but this is json
	im := eszip.GetImportMap("file:///import_map.json")
	if im == nil {
		t.Fatal("expected to find import map")
	}

	// Roundtrip
	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	module := parsed.GetModule("file:///import_map.json")
	if module == nil {
		t.Fatal("expected to find import map module after roundtrip")
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}
	if !strings.Contains(string(source), "imports") {
		t.Errorf("expected import map content, got %q", string(source))
	}
}

func TestAddOpaqueData(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddOpaqueData("data:///config", []byte("some binary data"))

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	module := parsed.GetModule("data:///config")
	if module == nil {
		t.Fatal("expected to find opaque data module")
	}
	if module.Kind != ModuleKindOpaqueData {
		t.Errorf("expected OpaqueData kind, got %v", module.Kind)
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}
	if string(source) != "some binary data" {
		t.Errorf("expected 'some binary data', got %q", string(source))
	}
}

func TestGetImportMapJsonc(t *testing.T) {
	eszip := NewV2()
	// Add a JSONC module - GetModule should NOT return it, GetImportMap should
	eszip.modules.Insert("file:///deno.jsonc", &ModuleData{
		Kind:      ModuleKindJsonc,
		Source:    NewReadySourceSlot([]byte(`{/* comment */ "imports":{}}`)),
		SourceMap: NewEmptySourceSlot(),
	})

	module := eszip.GetModule("file:///deno.jsonc")
	if module != nil {
		t.Error("expected GetModule to return nil for JSONC")
	}

	im := eszip.GetImportMap("file:///deno.jsonc")
	if im == nil {
		t.Fatal("expected GetImportMap to return JSONC module")
	}
	if im.Kind != ModuleKindJsonc {
		t.Errorf("expected JSONC kind, got %v", im.Kind)
	}
}

// --- Version and magic tests ---

func TestVersionFromMagic(t *testing.T) {
	tests := []struct {
		magic   [8]byte
		version EszipVersion
		ok      bool
	}{
		{MagicV2, VersionV2, true},
		{MagicV2_1, VersionV2_1, true},
		{MagicV2_2, VersionV2_2, true},
		{MagicV2_3, VersionV2_3, true},
		{[8]byte{'N', 'O', 'T', 'M', 'A', 'G', 'I', 'C'}, 0, false},
	}

	for _, tt := range tests {
		v, ok := VersionFromMagic(tt.magic[:])
		if ok != tt.ok {
			t.Errorf("VersionFromMagic(%q): ok=%v, want %v", tt.magic, ok, tt.ok)
		}
		if ok && v != tt.version {
			t.Errorf("VersionFromMagic(%q): version=%v, want %v", tt.magic, v, tt.version)
		}
	}

	// Short magic
	_, ok := VersionFromMagic([]byte("short"))
	if ok {
		t.Error("expected false for short magic")
	}
}

func TestVersionToMagic(t *testing.T) {
	if VersionV2.ToMagic() != MagicV2 {
		t.Error("V2 magic mismatch")
	}
	if VersionV2_1.ToMagic() != MagicV2_1 {
		t.Error("V2.1 magic mismatch")
	}
	if VersionV2_2.ToMagic() != MagicV2_2 {
		t.Error("V2.2 magic mismatch")
	}
	if VersionV2_3.ToMagic() != MagicV2_3 {
		t.Error("V2.3 magic mismatch")
	}

	// Unknown version defaults to latest
	unknown := EszipVersion(99)
	if unknown.ToMagic() != MagicV2_3 {
		t.Error("unknown version should default to latest magic")
	}
}

func TestHasMagic(t *testing.T) {
	if !HasMagic(MagicV2_3[:]) {
		t.Error("expected HasMagic to be true for valid magic")
	}
	if HasMagic([]byte("short")) {
		t.Error("expected HasMagic to be false for short data")
	}
	if HasMagic([]byte("NOTMAGIC")) {
		t.Error("expected HasMagic to be false for invalid magic")
	}
}

func TestVersionSupportsNpm(t *testing.T) {
	if VersionV2.SupportsNpm() {
		t.Error("V2 should not support npm")
	}
	if !VersionV2_1.SupportsNpm() {
		t.Error("V2.1 should support npm")
	}
}

func TestVersionSupportsOptions(t *testing.T) {
	if VersionV2.SupportsOptions() {
		t.Error("V2 should not support options")
	}
	if VersionV2_1.SupportsOptions() {
		t.Error("V2.1 should not support options")
	}
	if !VersionV2_2.SupportsOptions() {
		t.Error("V2.2 should support options")
	}
}

// --- Checksum tests ---

func TestChecksumDigestSize(t *testing.T) {
	if ChecksumNone.DigestSize() != 0 {
		t.Error("None digest should be 0")
	}
	if ChecksumSha256.DigestSize() != 32 {
		t.Error("SHA256 digest should be 32")
	}
	if ChecksumXxh3.DigestSize() != 8 {
		t.Error("XXH3 digest should be 8")
	}
	if ChecksumType(99).DigestSize() != 0 {
		t.Error("unknown checksum digest should be 0")
	}
}

func TestChecksumHash(t *testing.T) {
	data := []byte("test data")

	// None returns nil
	if ChecksumNone.Hash(data) != nil {
		t.Error("None hash should be nil")
	}

	// SHA256 returns 32 bytes
	sha := ChecksumSha256.Hash(data)
	if len(sha) != 32 {
		t.Errorf("SHA256 hash should be 32 bytes, got %d", len(sha))
	}

	// XXH3 returns 8 bytes
	xxh := ChecksumXxh3.Hash(data)
	if len(xxh) != 8 {
		t.Errorf("XXH3 hash should be 8 bytes, got %d", len(xxh))
	}

	// Unknown returns nil
	if ChecksumType(99).Hash(data) != nil {
		t.Error("unknown checksum hash should be nil")
	}
}

func TestChecksumVerify(t *testing.T) {
	data := []byte("test data")

	// None always verifies
	if !ChecksumNone.Verify(data, nil) {
		t.Error("None should always verify")
	}

	// SHA256 verify
	sha := ChecksumSha256.Hash(data)
	if !ChecksumSha256.Verify(data, sha) {
		t.Error("SHA256 should verify correct hash")
	}
	if ChecksumSha256.Verify(data, []byte("wrong")) {
		t.Error("SHA256 should not verify wrong hash")
	}

	// XXH3 verify
	xxh := ChecksumXxh3.Hash(data)
	if !ChecksumXxh3.Verify(data, xxh) {
		t.Error("XXH3 should verify correct hash")
	}
}

func TestChecksumFromU8(t *testing.T) {
	tests := []struct {
		b    uint8
		want ChecksumType
		ok   bool
	}{
		{0, ChecksumNone, true},
		{1, ChecksumSha256, true},
		{2, ChecksumXxh3, true},
		{3, ChecksumNone, false},
		{255, ChecksumNone, false},
	}

	for _, tt := range tests {
		got, ok := ChecksumFromU8(tt.b)
		if ok != tt.ok {
			t.Errorf("ChecksumFromU8(%d): ok=%v, want %v", tt.b, ok, tt.ok)
		}
		if got != tt.want {
			t.Errorf("ChecksumFromU8(%d): got=%v, want %v", tt.b, got, tt.want)
		}
	}
}

// --- ModuleKind string ---

func TestModuleKindUnknown(t *testing.T) {
	unknown := ModuleKind(99)
	if unknown.String() != "unknown" {
		t.Errorf("expected 'unknown', got %q", unknown.String())
	}
}

// --- npm package ID tests ---

func TestNpmPackageIDString(t *testing.T) {
	id := &NpmPackageID{Name: "@types/node", Version: "18.0.0"}
	if id.String() != "@types/node@18.0.0" {
		t.Errorf("expected '@types/node@18.0.0', got %q", id.String())
	}
}

func TestParseNpmPackageID(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		version string
		wantErr bool
	}{
		{"lodash@4.17.21", "lodash", "4.17.21", false},
		{"@types/node@18.0.0", "@types/node", "18.0.0", false},
		{"invalid", "", "", true},
		{"@scoped", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, err := ParseNpmPackageID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id.Name != tt.name {
				t.Errorf("expected name %q, got %q", tt.name, id.Name)
			}
			if id.Version != tt.version {
				t.Errorf("expected version %q, got %q", tt.version, id.Version)
			}
		})
	}
}

// --- npm snapshot roundtrip ---

func TestNpmSnapshotRoundtrip(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddModule("file:///main.js", ModuleKindJavaScript, []byte("import 'lodash'"), nil)

	// Create npm snapshot
	lodashID := &NpmPackageID{Name: "lodash", Version: "4.17.21"}
	eszip.npmSnapshot = &NpmResolutionSnapshot{
		Packages: []*NpmPackage{
			{
				ID:           lodashID,
				Dependencies: map[string]*NpmPackageID{},
			},
		},
		RootPackages: map[string]*NpmPackageID{
			"lodash": lodashID,
		},
	}

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	v2, ok := parsed.V2()
	if !ok {
		t.Fatal("expected V2")
	}

	snapshot := v2.TakeNpmSnapshot()
	if snapshot == nil {
		t.Fatal("expected npm snapshot")
	}

	if len(snapshot.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(snapshot.Packages))
	}

	pkg := snapshot.Packages[0]
	if pkg.ID.Name != "lodash" {
		t.Errorf("expected 'lodash', got %q", pkg.ID.Name)
	}
	if pkg.ID.Version != "4.17.21" {
		t.Errorf("expected '4.17.21', got %q", pkg.ID.Version)
	}

	// TakeNpmSnapshot again should return nil
	snapshot2 := v2.TakeNpmSnapshot()
	if snapshot2 != nil {
		t.Error("expected nil on second take")
	}
}

func TestNpmSnapshotWithDependencies(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddModule("file:///main.js", ModuleKindJavaScript, []byte("code"), nil)

	depID := &NpmPackageID{Name: "has-symbols", Version: "1.0.3"}
	mainID := &NpmPackageID{Name: "lodash", Version: "4.17.21"}

	eszip.npmSnapshot = &NpmResolutionSnapshot{
		Packages: []*NpmPackage{
			{
				ID:           depID,
				Dependencies: map[string]*NpmPackageID{},
			},
			{
				ID: mainID,
				Dependencies: map[string]*NpmPackageID{
					"has-symbols": depID,
				},
			},
		},
		RootPackages: map[string]*NpmPackageID{
			"lodash": mainID,
		},
	}

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	v2, ok := parsed.V2()
	if !ok {
		t.Fatal("expected V2")
	}

	snapshot := v2.TakeNpmSnapshot()
	if snapshot == nil {
		t.Fatal("expected npm snapshot")
	}

	if len(snapshot.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(snapshot.Packages))
	}

	if len(snapshot.RootPackages) != 1 {
		t.Fatalf("expected 1 root package, got %d", len(snapshot.RootPackages))
	}
}

// --- Parse existing test fixtures ---

func TestParseJsonEszip(t *testing.T) {
	data, err := os.ReadFile("testdata/json.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse json.eszip2: %v", err)
	}

	if !eszip.IsV2() {
		t.Fatal("expected V2 eszip")
	}

	specs := eszip.Specifiers()
	if len(specs) == 0 {
		t.Fatal("expected at least one specifier")
	}

	// Verify we can read all modules
	for _, spec := range specs {
		module := eszip.GetModule(spec)
		if module == nil {
			continue // Could be redirect
		}
		_, err := module.Source(ctx)
		if err != nil {
			t.Errorf("failed to get source for %s: %v", spec, err)
		}
	}
}

func TestParseWasmEszip(t *testing.T) {
	data, err := os.ReadFile("testdata/wasm.eszip2_3")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse wasm.eszip2_3: %v", err)
	}

	if !eszip.IsV2() {
		t.Fatal("expected V2 eszip")
	}

	specs := eszip.Specifiers()
	if len(specs) == 0 {
		t.Fatal("expected at least one specifier")
	}

	// Look for a wasm module
	foundWasm := false
	for _, spec := range specs {
		module := eszip.GetModule(spec)
		if module == nil {
			continue
		}
		if module.Kind == ModuleKindWasm {
			foundWasm = true
			source, err := module.Source(ctx)
			if err != nil {
				t.Errorf("failed to get wasm source: %v", err)
			}
			if len(source) == 0 {
				t.Error("expected non-empty wasm source")
			}
		}
	}

	if !foundWasm {
		t.Error("expected to find at least one wasm module")
	}
}

// --- V2 module kind roundtrip ---

func TestAllModuleKindsRoundtrip(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("js"), nil)
	eszip.AddModule("file:///test.json", ModuleKindJson, []byte(`{"a":1}`), nil)
	eszip.AddModule("file:///test.wasm", ModuleKindWasm, []byte{0x00, 0x61, 0x73, 0x6d}, nil)
	eszip.AddOpaqueData("data:///config", []byte("opaque"))

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	tests := []struct {
		specifier string
		kind      ModuleKind
		source    string
	}{
		{"file:///test.js", ModuleKindJavaScript, "js"},
		{"file:///test.json", ModuleKindJson, `{"a":1}`},
		{"file:///test.wasm", ModuleKindWasm, string([]byte{0x00, 0x61, 0x73, 0x6d})},
		{"data:///config", ModuleKindOpaqueData, "opaque"},
	}

	for _, tt := range tests {
		module := parsed.GetModule(tt.specifier)
		if module == nil {
			t.Errorf("expected to find module %s", tt.specifier)
			continue
		}
		if module.Kind != tt.kind {
			t.Errorf("%s: expected kind %v, got %v", tt.specifier, tt.kind, module.Kind)
		}
		source, err := module.Source(ctx)
		if err != nil {
			t.Errorf("%s: failed to get source: %v", tt.specifier, err)
			continue
		}
		if string(source) != tt.source {
			t.Errorf("%s: expected source %q, got %q", tt.specifier, tt.source, string(source))
		}
	}
}

// --- V2 redirect cycle detection ---

func TestV2RedirectCycle(t *testing.T) {
	eszip := NewV2()
	eszip.AddRedirect("file:///a.js", "file:///b.js")
	eszip.AddRedirect("file:///b.js", "file:///a.js")

	module := eszip.GetModule("file:///a.js")
	if module != nil {
		t.Error("expected nil for redirect cycle")
	}
}

// --- V2 GetModule for npm specifier ---

func TestV2GetModuleNpmSpecifier(t *testing.T) {
	eszip := NewV2()
	eszip.modules.Insert("npm:lodash", &NpmSpecifierEntry{PackageID: 0})

	module := eszip.GetModule("npm:lodash")
	if module != nil {
		t.Error("expected nil for npm specifier via GetModule")
	}
}

// --- Options/GetChecksumSize ---

func TestOptionsGetChecksumSize(t *testing.T) {
	// When ChecksumSize is set, use it
	opts := Options{Checksum: ChecksumSha256, ChecksumSize: 16}
	if opts.GetChecksumSize() != 16 {
		t.Errorf("expected 16, got %d", opts.GetChecksumSize())
	}

	// When ChecksumSize is 0, use digest size
	opts2 := Options{Checksum: ChecksumSha256, ChecksumSize: 0}
	if opts2.GetChecksumSize() != 32 {
		t.Errorf("expected 32, got %d", opts2.GetChecksumSize())
	}
}

func TestDefaultOptionsForVersion(t *testing.T) {
	// V2 and V2_1 default to SHA256
	optsV2 := DefaultOptionsForVersion(VersionV2)
	if optsV2.Checksum != ChecksumSha256 {
		t.Errorf("expected SHA256 for V2, got %v", optsV2.Checksum)
	}
	optsV2_1 := DefaultOptionsForVersion(VersionV2_1)
	if optsV2_1.Checksum != ChecksumSha256 {
		t.Errorf("expected SHA256 for V2.1, got %v", optsV2_1.Checksum)
	}

	// V2.2+ default to None
	optsV2_2 := DefaultOptionsForVersion(VersionV2_2)
	if optsV2_2.Checksum != ChecksumNone {
		t.Errorf("expected None for V2.2, got %v", optsV2_2.Checksum)
	}
}

// --- ParseV2 and ParseV2Sync (public wrappers) ---

func TestParseV2Direct(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseV2Sync(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	module := eszip.GetModule("file:///main.ts")
	if module == nil {
		t.Fatal("expected to find module")
	}
}

func TestParseV2WithInvalidMagic(t *testing.T) {
	ctx := context.Background()
	_, _, err := ParseV2(ctx, bytes.NewReader([]byte("NOTMAGIC")))
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}

func TestParseV2Streaming(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, complete, err := ParseV2(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Before completing, the module structure should be there
	specs := eszip.Specifiers()
	if len(specs) == 0 {
		t.Fatal("expected specifiers before completion")
	}

	// Complete the parse
	if err := complete(ctx); err != nil {
		t.Fatalf("failed to complete: %v", err)
	}

	module := eszip.GetModule("file:///main.ts")
	if module == nil {
		t.Fatal("expected to find module after completion")
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}
	if len(source) == 0 {
		t.Error("expected non-empty source")
	}
}

// --- Section tests ---

func TestSectionMethods(t *testing.T) {
	s := &Section{
		content:  []byte("hello"),
		hash:     []byte("hash"),
		checksum: ChecksumNone,
	}

	if string(s.Content()) != "hello" {
		t.Error("content mismatch")
	}
	if s.ContentLen() != 5 {
		t.Errorf("expected content len 5, got %d", s.ContentLen())
	}
	if s.TotalLen() != 9 {
		t.Errorf("expected total len 9, got %d", s.TotalLen())
	}
	if !s.IsChecksumValid() {
		t.Error("None checksum should always be valid")
	}

	content := s.IntoContent()
	if string(content) != "hello" {
		t.Error("IntoContent mismatch")
	}
	if s.Content() != nil {
		t.Error("expected nil content after IntoContent")
	}
}

// --- Empty module (zero-length source) roundtrip ---

func TestEmptySourceRoundtrip(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.AddModule("file:///empty.js", ModuleKindJavaScript, []byte{}, nil)

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := ParseBytes(ctx, data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	module := parsed.GetModule("file:///empty.js")
	if module == nil {
		t.Fatal("expected to find module")
	}

	source, err := module.Source(ctx)
	if err != nil {
		t.Fatalf("failed to get source: %v", err)
	}
	if len(source) != 0 {
		t.Errorf("expected empty source, got %d bytes", len(source))
	}
}

// --- ParseSync tests ---

func TestParseSyncV1(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseSync(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if !eszip.IsV1() {
		t.Fatal("expected V1")
	}
}

func TestParseSyncV2(t *testing.T) {
	data, err := os.ReadFile("testdata/redirect.eszip2")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	ctx := context.Background()
	eszip, err := ParseSync(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if !eszip.IsV2() {
		t.Fatal("expected V2")
	}
}

// --- Manual corrupt archive for specific error paths ---

func TestParseV2InvalidEntryKind(t *testing.T) {
	ctx := context.Background()

	// Build a minimal V2.2 archive with an invalid entry kind in the modules header
	eszip := NewV2()
	eszip.SetChecksum(ChecksumNone)
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("x"), nil)

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	// Find and corrupt the entry kind byte in the modules header.
	// Structure: magic(8) + opts_len(4) + opts(4) + modules_len(4) + modules_header...
	// In the modules header: specifier_len(4) + specifier(n) + entry_kind(1) + ...
	offset := 8 + 4 + 4 + 4 // magic + opts_len + opts + modules_len
	specLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	entryKindOffset := offset + 4 + specLen // after specifier_len + specifier

	if entryKindOffset < len(data) {
		data[entryKindOffset] = 99 // Invalid entry kind
	}

	_, err = ParseBytes(ctx, data)
	if err == nil {
		t.Fatal("expected error for invalid entry kind")
	}
	var pe *ParseError
	if errors.As(err, &pe) {
		if pe.Type != ErrInvalidV2EntryKind {
			t.Errorf("expected ErrInvalidV2EntryKind, got %v", pe.Type)
		}
	}
}

func TestParseV2InvalidModuleKind(t *testing.T) {
	ctx := context.Background()

	eszip := NewV2()
	eszip.SetChecksum(ChecksumNone)
	eszip.AddModule("file:///test.js", ModuleKindJavaScript, []byte("x"), nil)

	data, err := eszip.IntoBytes()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	// Find the module kind byte (last byte of the module entry in the header).
	// After entry_kind(1=module), we have: source_offset(4) + source_len(4) + srcmap_offset(4) + srcmap_len(4) + module_kind(1)
	offset := 8 + 4 + 4 + 4
	specLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	moduleKindOffset := offset + 4 + specLen + 1 + 16 // +1 entry_kind, +16 (4x uint32)

	if moduleKindOffset < len(data) {
		data[moduleKindOffset] = 99 // Invalid module kind
	}

	_, err = ParseBytes(ctx, data)
	if err == nil {
		t.Fatal("expected error for invalid module kind")
	}
	var pe *ParseError
	if errors.As(err, &pe) {
		if pe.Type != ErrInvalidV2ModuleKind {
			t.Errorf("expected ErrInvalidV2ModuleKind, got %v", pe.Type)
		}
	}
}
