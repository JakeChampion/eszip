# eszip

A Go library for serializing and deserializing ECMAScript module graphs into
a compact binary format (eszip). The eszip format is designed to be compact and
streaming-capable, allowing efficient storage and loading of large JavaScript
and TypeScript module collections.

## Installation

```shell
go get github.com/aspect-build/eszip
```

## Library usage

### Parsing an eszip archive

```go
import (
    "context"
    "os"

    "github.com/aspect-build/eszip"
)

data, _ := os.ReadFile("archive.eszip2")
archive, _ := eszip.ParseBytes(context.Background(), data)

for _, spec := range archive.Specifiers() {
    module := archive.GetModule(spec)
    source, _ := module.Source(context.Background())
    fmt.Printf("%s: %d bytes\n", spec, len(source))
}
```

### Creating an eszip archive

```go
archive := eszip.NewV2()
archive.SetChecksum(eszip.ChecksumSha256)
archive.AddModule("file:///main.js", eszip.ModuleKindJavaScript, sourceBytes, nil)

data, _ := archive.IntoBytes()
os.WriteFile("output.eszip2", data, 0644)
```

## CLI tool

Build the CLI:

```shell
go build -o eszip ./cmd/eszip
```

### Commands

```
eszip view archive.eszip2              # View contents
eszip view -s file:///main.ts archive  # View specific module
eszip extract -o ./output archive      # Extract to disk
eszip create -o archive.eszip2 *.js    # Create from files
eszip info archive.eszip2              # Show archive metadata
```

## File format

```
Eszip:
| Magic (8) | Header size (4) | Header (n) | Header hash (32) | Sources size (4) | Sources (n) | SourceMaps size (4) | SourceMaps (n) |

Header:
( | Specifier size (4) | Specifier (n) | Entry type (1) | Entry (n) | )*

Entry (redirect):
| Specifier size (4) | Specifier (n) |

Entry (module):
| Source offset (4) | Source size (4) | SourceMap offset (4) | SourceMap size (4) | Module type (1) |

Sources:
( | Source (n) | Hash (32) | )*

SourceMaps:
( | SourceMap (n) | Hash (32) | )*
```

If both the offset and size for a source or source map are 0, no entry and no
hash is present in the data sections for that module.

## License

MIT
