# go-winmd

[![Go Reference](https://pkg.go.dev/badge/github.com/deploymenttheory/go-winmd.svg)](https://pkg.go.dev/github.com/deploymenttheory/go-winmd)
[![CI](https://github.com/deploymenttheory/go-winmd/actions/workflows/ci.yml/badge.svg)](https://github.com/deploymenttheory/go-winmd/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A native Go reader for ECMA-335 metadata files (`.winmd`), aligned with the
**ECMA-335 6th edition** standard. Standard library only — no .NET, no cgo,
no dependencies.

This is the shared foundation of the deploymenttheory Windows bindings
family: [go-bindings-win32](https://github.com/deploymenttheory/go-bindings-win32),
go-bindings-wdk, and (planned) go-bindings-winrt all generate from metadata
parsed by this module.

## What it does

- **PE container → CLI metadata root → heaps → tables**: parses `#~` and
  `#-` table streams, `#Strings`/`#Blob`/`#GUID` heaps, with every exported
  symbol carrying its §II.x specification reference.
- **All 45 ECMA-335 tables** sized and skipped correctly; the 15 tables the
  Windows metadata projections need are materialized into typed rows, with
  typed `Table` IDs and typed bitmask columns (`TypeAttributes`,
  `ParamAttributes`, `PInvokeAttributes`, …) in specification vocabulary.
- **Signature blobs** (`MethodDefSig`, `FieldSig`, §II.23.2) decoded into a
  recursive `TypeSig` grammar.
- **Custom-attribute values decoded** (§II.23.3) — fixed and named arguments,
  not just raw blobs — plus `Constant`-table value decoding. These are the
  pieces most winmd readers omit.
- **Hardened against hostile input**: untrusted lengths and row indices are
  bounds-checked and allocation-clamped; corrupt files return errors, never
  panic or over-allocate.

Tested by brute force: every one of the ~318k signatures and ~152k custom
attributes in the pinned `Windows.Win32.winmd` must decode with zero
failures (`testdata/PROVENANCE.json` pins the fixture; it is fetched on
demand and sha256-verified).

## Usage

```go
import "github.com/deploymenttheory/go-winmd"

file, err := winmd.Open("Windows.Win32.winmd")
if err != nil { /* ... */ }

for i := range file.Tables.TypeDefs {
    td := &file.Tables.TypeDefs[i]
    if td.Flags&winmd.TypeAttrInterface != 0 {
        fmt.Println(td.Namespace, td.Name, "COM interface")
    }
}

sig, err := file.MethodSignature(file.Tables.Methods[0].Signature)
attrs := file.AttributesFor(winmd.CodedIndex{Table: winmd.TableTypeDef, Row: 1})
```

The `nuget` subpackage downloads winmd files from NuGet (flat-container API)
with provenance records — used by the bindings generators' `fetch-metadata`
commands and by this module's own test fixture.

## Non-goals

Deliberately scoped to what the Windows metadata projections need (recorded
in the package documentation): no lazy per-row table access, no generic
coded-index tag types, no `#US` heap, no generics signature decoding (yet —
it lands here when go-bindings-winrt needs it; the Win32/WDK metadata
contains none).

## Documentation

- [Getting started](docs/getting-started.md) — open a file, iterate tables,
  decode signatures and attributes
- [ECMA-335 notes](docs/ecma335-notes.md) — materialized vs sized-only tables,
  the non-goals, comparison vs microsoft/go-winmd
- [`CLAUDE.md`](CLAUDE.md) — the as-built architecture

## License

[MIT](LICENSE).
