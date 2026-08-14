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

go run ./cmd/winmd-update -check   # report upstream metadata drift
go run ./cmd/winmd-update          # bump the pins in testdata/PROVENANCE.json
```

The test fixtures — `Windows.Win32.winmd` (Win32 metadata) and
`Windows.Foundation.UniversalApiContract.winmd` (WinRT metadata, from the
`Microsoft.Windows.SDK.Contracts` package) — are pinned by
`testdata/PROVENANCE.json` (version + sha256), fetched on demand via the
`pkg/nuget` subpackage into a gitignored `testdata/` path and sha256-verified.
Offline runs skip.

## Architecture

```
pkg/winmd/           the reader (import path .../go-winmd/pkg/winmd)
pkg/nuget/           NuGet flat-container fetch + provenance records
cmd/winmd-update/    refreshes the testdata/PROVENANCE.json pins
testdata/            PROVENANCE.json; the .winmd files are gitignored
```

Inside `pkg/winmd/`:

```
PE container → CLI metadata root → heaps → tables → signatures / attributes
 (winmd.go)      (winmd.go)      (heaps.go) (tables.go)  (sig.go / attrs.go)
```

- **`pkg/winmd/winmd.go`** — `Open`/`Parse`; PE walk via `debug/pe`; CLI (COR20) header;
  metadata root + stream headers (`#~`/`#-`, `#Strings`, `#Blob`, `#GUID`).
- **`pkg/winmd/heaps.go`** — `StringHeap`/`GUIDHeap`/`BlobHeap` + the `blobReader`
  cursor (compressed-int decoding, sticky-error model).
- **`pkg/winmd/tables.go`** — the typed `Table` enum (all 45 tables, spec names), the
  static `tableSchemas` column layout (§II.22) that sizes every table, coded
  indices resolved eagerly to `{Table, Row}`, and the 22 materialized tables'
  `*Row` structs decoded into eager slices (including the WinRT
  event/property tables: `Event`/`EventMap`, `Property`/`PropertyMap`,
  `MethodSemantics`).
- **`pkg/winmd/flags.go`** — typed bitmask columns (`TypeAttributes`, `FieldAttributes`,
  `MethodAttributes`, `ParamAttributes`, `PInvokeAttributes`,
  `EventAttributes`, `PropertyAttributes`, `MethodSemanticsAttributes`, …,
  §II.23.1) with spec member names and `String()` methods.
- **`pkg/winmd/sig.go`** — `MethodSignature`/`FieldSignature`/`PropertySignature` → the
  recursive `TypeSig` grammar (§II.23.2), including generics
  (GENERICINST/VAR/MVAR).
- **`pkg/winmd/attrs.go`** — `AttributesFor` decodes custom-attribute **values** (fixed +
  named args, §II.23.3), not just raw blobs.
- **`pkg/winmd/constants.go`** — `ElementType` (Go-idiomatic names; spec `ELEMENT_TYPE_*`
  in the doc comments) and `DecodeConstant` for Constant-table blobs.
- **`pkg/nuget/`** — stdlib-only NuGet flat-container fetch + provenance records;
  used by the bindings generators' `fetch-metadata` and this module's fixture.
  `nuget.go` is the single-file path (`Fetch`/`ExtractFile`), one download per
  file. `multifile.go` serves meta-packages, where that model breaks down:
  `Dependencies`/`ParseDependencies` read a nuspec's fan-out (the Windows App
  SDK is nine component packages whose set and versions move every servicing
  release, so it must be discovered, not hard-coded), and
  `FetchArchive` + `ExtractMatching`/`EntryNames` + `ProvenanceFor` split the
  download from the extraction so one archive yields many files and many
  provenance records. Entry keys are full archive paths, never base names —
  a nupkg routinely ships the same file under several architectures.
- **`cmd/winmd-update/`** — resolves the newest published version of each
  `testdata/PROVENANCE.json` pin, fetches it into `testdata/` and rewrites
  the record. A pin on a prerelease tracks prereleases, a stable pin tracks
  stable only, and `isAhead` stops a pin ever being walked backwards. Driven
  weekly by `.github/workflows/metadata-update.yml`, which runs the decode
  suite against the *new* metadata and opens the bump as a PR — draft, with
  the failing output in the body, when the new metadata does not decode.

## Spec alignment

Every exported symbol carries its ECMA-335 6th-edition `§II.x` reference. Table
IDs and flag columns are typed with spec member names. Untrusted lengths and
row indices are bounds-checked and allocation-clamped (`pkg/winmd/corrupt_test.go`);
corrupt files return structured errors, never panic or over-allocate.

## Non-goals (see the package doc in `pkg/winmd/winmd.go`)

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
