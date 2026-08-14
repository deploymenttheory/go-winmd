# go-winmd

[![GoDoc](https://pkg.go.dev/badge/github.com/deploymenttheory/go-winmd)](https://pkg.go.dev/github.com/deploymenttheory/go-winmd)
[![License](https://img.shields.io/github/license/deploymenttheory/go-winmd)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/deploymenttheory/go-winmd)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/deploymenttheory/go-winmd)](https://github.com/deploymenttheory/go-winmd/releases)
[![codecov](https://codecov.io/gh/deploymenttheory/go-winmd/graph/badge.svg)](https://codecov.io/gh/deploymenttheory/go-winmd)
![Status: GA](https://img.shields.io/badge/status-GA-green)

A native Go reader for ECMA-335 metadata files (`.winmd`), aligned with the
**ECMA-335 6th edition** standard. Standard library only — no .NET, no cgo,
no dependencies.

This is the shared foundation of the deploymenttheory Windows bindings
family: [go-bindings-win32](https://github.com/deploymenttheory/go-bindings-win32),
[go-bindings-wdk](https://github.com/deploymenttheory/go-bindings-wdk),
[go-bindings-wmi](https://github.com/deploymenttheory/go-bindings-wmi) and
[go-bindings-winrt](https://github.com/deploymenttheory/go-bindings-winrt)
all generate from metadata parsed by this module. Changing it changes all of
them.

## What it does

- **PE container → CLI metadata root → heaps → tables**: parses `#~` and
  `#-` table streams, `#Strings`/`#Blob`/`#GUID` heaps, with every exported
  symbol carrying its §II.x specification reference.
- **All 45 ECMA-335 tables** sized and skipped correctly; the 22 tables the
  Windows metadata projections need are materialized into typed rows, with
  typed `Table` IDs and typed bitmask columns (`TypeAttributes`,
  `ParamAttributes`, `PInvokeAttributes`, …) in specification vocabulary.
- **Signature blobs** (`MethodDefSig`, `FieldSig`, `PropertySig`, §II.23.2)
  decoded into a recursive `TypeSig` grammar, generics included.
- **Custom-attribute values decoded** (§II.23.3) — fixed and named arguments,
  not just raw blobs — plus `Constant`-table value decoding. These are the
  pieces most winmd readers omit.
- **Hardened against hostile input**: untrusted lengths and row indices are
  bounds-checked and allocation-clamped; corrupt files return errors, never
  panic or over-allocate.

Tested by brute force against two real winmds: every one of the ~318k
signatures and ~152k custom attributes in `Windows.Win32.winmd`, and the
~73k signatures, ~31k property signatures and ~56k attributes in the WinRT
`Windows.Foundation.UniversalApiContract.winmd`, must decode with zero
failures. `testdata/PROVENANCE.json` pins both fixtures by version and
sha256; they are fetched on demand and verified, and offline runs skip.

## Layout

```text
pkg/winmd/           the reader — PE → CLI header → heaps → tables → signatures
pkg/nuget/           stdlib-only NuGet flat-container fetch + provenance records
cmd/winmd-update/    refreshes the pinned metadata in testdata/PROVENANCE.json
testdata/            PROVENANCE.json pins; the .winmd files are fetched on demand
```

## Usage

```go
import "github.com/deploymenttheory/go-winmd/pkg/winmd"

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

The `pkg/nuget` subpackage downloads winmd files from NuGet (flat-container
API) with provenance records — used by the bindings generators'
`fetch-metadata` commands and by this module's own test fixture.

## Keeping the metadata current

`testdata/PROVENANCE.json` pins each upstream NuGet package by version and
sha256. `cmd/winmd-update` resolves the newest published version of each pin,
fetches it and rewrites the record:

```sh
go run ./cmd/winmd-update -check   # report what has moved upstream
go run ./cmd/winmd-update          # bump the pins and cache the new files
```

A pin on a prerelease (`Windows.Win32.winmd` ships as `-preview`) tracks
prereleases; a pin on a stable version tracks stable versions only, and a pin
is never walked backwards.

The [Metadata Update](.github/workflows/metadata-update.yml) workflow runs
this weekly (and on manual dispatch). When a pin moves it runs the full
decode suite against the *new* metadata and opens a PR — as a draft, with the
failing test output in the body, if the new metadata does not decode cleanly.

## Non-goals

Deliberately scoped to what the Windows metadata projections need (recorded
in the package documentation): no lazy per-row table access, no generic
coded-index tag types, no `#US` heap, no BYREF/multi-rank-array signature
decoding. Generics and the WinRT event/property tables ARE decoded (for
go-bindings-winrt); tripwire tests prove the Win32/WDK metadata contains
neither, so those projections are unaffected.

## Documentation

- [Getting started](docs/getting-started.md) — open a file, iterate tables,
  decode signatures and attributes
- [ECMA-335 notes](docs/ecma335-notes.md) — materialized vs sized-only tables,
  the non-goals, comparison vs microsoft/go-winmd
- [`CLAUDE.md`](CLAUDE.md) — the as-built architecture

## Related projects

Part of the deploymenttheory Windows bindings family:

- **go-winmd** — the shared ECMA-335 `.winmd` metadata reader *(this repo)*
- [go-bindings-win32](https://github.com/deploymenttheory/go-bindings-win32) — the Win32 API surface — functions, structs, enums, COM
- [go-bindings-wdk](https://github.com/deploymenttheory/go-bindings-wdk) — the Windows Driver Kit / user-mode Native API surface
- [go-bindings-wmi](https://github.com/deploymenttheory/go-bindings-wmi) — typed WMI/CIM classes
- [go-bindings-winrt](https://github.com/deploymenttheory/go-bindings-winrt) — WinRT bindings (in progress)

## License

[MIT](LICENSE).
