# CLAUDE.md

Guidance for Claude Code (claude.ai/code) working in this repository.

## What this is

`go-winmd` is a native Go reader for ECMA-335 metadata files (`.winmd`),
aligned with the **ECMA-335 6th edition** standard. Standard library only — no
.NET, no cgo, no dependencies. It is the **shared foundation** of the
deploymenttheory Windows bindings family: `go-bindings-win32`,
`go-bindings-wdk`, and `go-bindings-winrt` all generate from metadata parsed
by this module. Changing it changes all of them.

## Commands

```sh
go build ./...
go vet ./...
go test ./...     # fetches the pinned winmd fixtures on first run
```

The test fixtures — `Windows.Win32.winmd` (Win32 metadata) and
`Windows.Foundation.UniversalApiContract.winmd` (WinRT metadata, from the
`Microsoft.Windows.SDK.Contracts` package) — are pinned by
`testdata/PROVENANCE.json` (version + sha256), fetched on demand via the
`nuget` subpackage into a gitignored `testdata/` path and sha256-verified.
Offline runs skip.

## Architecture

```
PE container → CLI metadata root → heaps → tables → signatures / attributes
 (winmd.go)      (winmd.go)      (heaps.go) (tables.go)  (sig.go / attrs.go)
```

- **`winmd.go`** — `Open`/`Parse`; PE walk via `debug/pe`; CLI (COR20) header;
  metadata root + stream headers (`#~`/`#-`, `#Strings`, `#Blob`, `#GUID`).
- **`heaps.go`** — `StringHeap`/`GUIDHeap`/`BlobHeap` + the `blobReader`
  cursor (compressed-int decoding, sticky-error model).
- **`tables.go`** — the typed `Table` enum (all 45 tables, spec names), the
  static `tableSchemas` column layout (§II.22) that sizes every table, coded
  indices resolved eagerly to `{Table, Row}`, and the 22 materialized tables'
  `*Row` structs decoded into eager slices (including the WinRT
  event/property tables: `Event`/`EventMap`, `Property`/`PropertyMap`,
  `MethodSemantics`).
- **`flags.go`** — typed bitmask columns (`TypeAttributes`, `FieldAttributes`,
  `MethodAttributes`, `ParamAttributes`, `PInvokeAttributes`,
  `EventAttributes`, `PropertyAttributes`, `MethodSemanticsAttributes`, …,
  §II.23.1) with spec member names and `String()` methods.
- **`sig.go`** — `MethodSignature`/`FieldSignature`/`PropertySignature` → the
  recursive `TypeSig` grammar (§II.23.2), including generics
  (GENERICINST/VAR/MVAR).
- **`attrs.go`** — `AttributesFor` decodes custom-attribute **values** (fixed +
  named args, §II.23.3), not just raw blobs.
- **`constants.go`** — `ElementType` (Go-idiomatic names; spec `ELEMENT_TYPE_*`
  in the doc comments) and `DecodeConstant` for Constant-table blobs.
- **`nuget/`** — stdlib-only NuGet flat-container fetch + provenance records;
  used by the bindings generators' `fetch-metadata` and this module's fixture.

## Spec alignment

Every exported symbol carries its ECMA-335 6th-edition `§II.x` reference. Table
IDs and flag columns are typed with spec member names. Untrusted lengths and
row indices are bounds-checked and allocation-clamped (`corrupt_test.go`);
corrupt files return structured errors, never panic or over-allocate.

## Non-goals (see the package doc in `winmd.go`)

Deliberately omitted, evaluated against `microsoft/go-winmd`: no lazy per-row
table access (the consumers scan every row), no generic `CodedIndex[T]` tag
types, no table-layout codegen, no `#US` heap, no BYREF/multi-rank signature
decoding (absent from all consumed winmds — such constructs error rather
than mis-decode). Generics and the event/property tables ARE decoded (added
as versioned additive changes for `go-bindings-winrt`); the
`TestWin32HasNoGenerics` and `TestWin32HasNoEventsOrProperties` tripwires
prove the Win32/WDK projections cannot observe them.

## Testing doctrine

The suites brute-force the entire pinned winmds: every signature and custom
attribute in both fixtures (~318k sigs + ~152k attrs in Win32; ~73k sigs +
~31k property sigs + ~56k attrs in the WinRT contract) must decode with
**zero failures**, plus golden spot-checks and hostile-input tests. This
"decode the whole real file" bar is the primary regression guard — keep it
at zero failures.
