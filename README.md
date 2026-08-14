# go-winmd

[![GoDoc](https://pkg.go.dev/badge/github.com/deploymenttheory/go-winmd)](https://pkg.go.dev/github.com/deploymenttheory/go-winmd)
[![License](https://img.shields.io/github/license/deploymenttheory/go-winmd)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/deploymenttheory/go-winmd)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/deploymenttheory/go-winmd)](https://github.com/deploymenttheory/go-winmd/releases)
[![codecov](https://codecov.io/gh/deploymenttheory/go-winmd/graph/badge.svg)](https://codecov.io/gh/deploymenttheory/go-winmd)
![Status: GA](https://img.shields.io/badge/status-GA-green)

A native Go reader for ECMA-335 metadata files (`.winmd`), aligned with the
**ECMA-335 6th edition** standard. It uses the standard library and nothing
else: no .NET, no cgo, no third-party dependencies.

## Why this exists

Microsoft publishes the Windows API surface as machine-readable metadata.
Every Win32 function, struct, enum, COM interface and constant is described
in a `.winmd` file that ships on NuGet and is versioned alongside the SDK.
That file records the things a C header leaves implicit: which DLL a
function is imported from, the GUID sitting on a COM interface, which
pointers are optional, which typedefs are genuinely distinct handle types
and which are just aliases. Generating Go bindings from metadata is what
keeps them correct as the SDK moves, instead of drifting the way
hand-written bindings always do.

The catch is that `.winmd` is a .NET file format. It is a PE image carrying
CLR metadata, meant to be read by .NET tooling, so reading one normally
means a .NET runtime or `System.Reflection.Metadata` somewhere in your
build. Putting a .NET install into the build pipeline of a Go project is a
poor trade, and reaching for a C library through cgo only swaps one build
dependency for another while making cross-compilation harder.

So this module reads the format directly. It walks the PE container, finds
the CLI header, and decodes the heaps, tables, signatures and custom
attributes itself, in Go, using the standard library alone. `go build` is
the entire toolchain.

Custom attributes are the part that matters most, and the part most readers
skip. The Windows-specific information is not in the tables, it is in
attribute values hung off them, so a reader that hands those back as raw
blobs has left the real work to whoever called it. This one decodes them,
and `Constant` table values with them. That is also why it does not build on
`microsoft/go-winmd`, which has no tagged releases, pulls in
`golang.org/x/tools`, and decodes neither attribute values, nor `Constant`
values, nor the `#-` stream variant that the Windows projections need.

This is the shared foundation of the deploymenttheory Windows bindings
family: [go-bindings-win32](https://github.com/deploymenttheory/go-bindings-win32),
[go-bindings-wdk](https://github.com/deploymenttheory/go-bindings-wdk),
[go-bindings-wmi](https://github.com/deploymenttheory/go-bindings-wmi) and
[go-bindings-winrt](https://github.com/deploymenttheory/go-bindings-winrt)
all generate from metadata parsed by this module. Changing it changes all of
them.

## What it does

It reads a `.winmd` file in the order the specification lays one out. First
the PE container, then the CLI metadata root, then the heaps, then the
tables. Both table stream formats are handled, `#~` and `#-`, along with the
`#Strings`, `#Blob` and `#GUID` heaps. Every exported symbol names the §II.x
section it comes from, so you can read the code next to the standard and
check one against the other.

All 45 ECMA-335 tables are sized correctly, which is what lets the reader
step over the ones it has no use for. The 22 tables the Windows metadata
projections actually need are decoded into typed rows. Table IDs are typed,
and so are the bitmask columns such as `TypeAttributes`, `ParamAttributes`
and `PInvokeAttributes`, each one named the way the specification names it.

Signature blobs decode into a recursive `TypeSig` grammar, generics
included. That covers `MethodDefSig`, `FieldSig` and `PropertySig` (§II.23.2).

Custom-attribute values are decoded rather than handed back as raw bytes
(§II.23.3), both the fixed arguments and the named ones, and `Constant`
table values are decoded too. Most winmd readers stop at the blob and leave
this part to you.

Hostile input is assumed throughout. Untrusted lengths and row indices are
bounds-checked and clamped before anything is allocated, so a corrupt file
comes back as an error rather than a panic or a request for several
gigabytes of memory.

## How it is tested

Two real winmd files are decoded end to end on every run, and everything in
them has to decode without a single failure. For `Windows.Win32.winmd` that
is roughly 318k signatures and 152k custom attributes. For the WinRT
`Windows.Foundation.UniversalApiContract.winmd` it is roughly 73k
signatures, 31k property signatures and 56k custom attributes.

`testdata/PROVENANCE.json` pins both files by version and sha256. Each one
is downloaded the first time a test needs it and checked against its pin, so
the suite always runs against known bytes. If there is no network, the tests
that need a fixture skip instead of failing.

## Layout

```text
pkg/winmd/           the reader: PE, CLI header, heaps, tables, signatures
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
API) with provenance records. The bindings generators use it for their
`fetch-metadata` commands, and this module uses it for its own test fixtures.

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
decode suite against the *new* metadata and opens a PR. If the new metadata
does not decode cleanly the PR is opened as a draft, with the failing test
output in the body.

## Non-goals

Deliberately scoped to what the Windows metadata projections need (recorded
in the package documentation): no lazy per-row table access, no generic
coded-index tag types, no `#US` heap, no BYREF/multi-rank-array signature
decoding. Generics and the WinRT event/property tables ARE decoded (for
go-bindings-winrt); tripwire tests prove the Win32/WDK metadata contains
neither, so those projections are unaffected.

## Documentation

- [Getting started](docs/getting-started.md): open a file, iterate tables,
  decode signatures and attributes
- [ECMA-335 notes](docs/ecma335-notes.md): materialized versus sized-only
  tables, the non-goals, and a comparison with microsoft/go-winmd
- [`CLAUDE.md`](CLAUDE.md): the as-built architecture

## Related projects

Part of the deploymenttheory Windows bindings family:

- **go-winmd**: the shared ECMA-335 `.winmd` metadata reader *(this repo)*
- [go-bindings-win32](https://github.com/deploymenttheory/go-bindings-win32): the Win32 API surface, covering functions, structs, enums and COM
- [go-bindings-wdk](https://github.com/deploymenttheory/go-bindings-wdk): the Windows Driver Kit and user-mode Native API surface
- [go-bindings-wmi](https://github.com/deploymenttheory/go-bindings-wmi): typed WMI and CIM classes
- [go-bindings-winrt](https://github.com/deploymenttheory/go-bindings-winrt): WinRT bindings, in progress

## License

[MIT](LICENSE).
