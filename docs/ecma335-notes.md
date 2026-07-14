# ECMA-335 notes

`go-winmd` is aligned with **ECMA-335 6th edition, partition II**. Every
exported symbol's doc comment cites its section (`§II.22` tables, `§II.23`
blobs/flags/signatures, `§II.24` physical layout, `§II.25` PE format).

## Materialized vs sized-only tables

All 45 tables are transcribed in `tableSchemas` so that **every** table's rows
can be sized correctly (needed to step over tables and to compute coded-index
widths). Only the 15 tables the Windows projections consume are decoded into
typed `*Row` slices:

> TypeRef, TypeDef, Field, MethodDef, Param, InterfaceImpl, MemberRef,
> Constant, CustomAttribute, ClassLayout, FieldLayout, ModuleRef, TypeSpec,
> ImplMap, NestedClass.

The other 30 are stepped over (their sizes are known; their rows are not built).

## Heaps

`#~` and `#-` (the uncompressed variant) both feed the table decoder;
`#Strings`, `#Blob`, `#GUID` are exposed as typed heaps. `#US` (user strings)
is recognized and skipped — IL plumbing that winmd projections never reference.

## What is intentionally not supported

These are deliberate non-goals (see the package doc), not gaps:

- **No lazy row access.** Consumers scan every row of every materialized table,
  so eager typed slices are simpler and faster than an on-demand `At(row)` API.
- **No generic `CodedIndex[T]` tag types.** Coded indices resolve eagerly to a
  concrete `{Table, Row}` pair — strictly more informative than a tag+index.
- **No table-layout code generation.** `tableSchemas` is the hand-transcribed
  §II.22 layout; a schema-consistency test guards transcription errors.
- **No BYREF / multi-rank arrays in signatures.** The Win32 and WDK winmds
  contain none (the brute-force suites assert this), so such constructs return
  a structured error rather than being silently mis-decoded.

## Generics (the WinRT foundation)

Generics **are** decoded, as the shared foundation for `go-bindings-winrt`:

- Signatures: `GENERICINST` → `SigGenericInst` (with `GenericArgs`), `VAR` →
  `SigVar`, `MVAR` → `SigMVar` (with `GenericIndex`), all in `TypeSig`.
- Tables: `GenericParam` (§II.22.20) and `GenericParamConstraint` (§II.22.21)
  are materialized; `TypeSpecSignature` decodes a TypeSpec blob (§II.23.2.14).

The Win32 and WDK winmds contain **zero** generic constructs — `TestWin32HasNoGenerics`
asserts this — so this support is inert for those projections and their
generated output is unaffected. It exists for WinRT metadata (`IVector<T>`,
`IAsyncOperation<T>`, parameterized delegates/events). The WinRT *emitter* and
*runtime* (HSTRING, IInspectable, activation) live in `go-bindings-winrt`;
this is only the reader layer.

## Events and properties (the WinRT member model)

The WinRT member tables **are** materialized, completing the reader
prerequisites for `go-bindings-winrt`:

- Tables: `Event`/`EventMap` (§II.22.13/12), `Property`/`PropertyMap`
  (§II.22.34/35), and `MethodSemantics` (§II.22.28) binding `get_`/`put_`/
  `add_`/`remove_` accessor methods to their property or event. The map
  tables carry 1-based half-open ranges (`EventFirst`/`EventEnd`,
  `PropertyFirst`/`PropertyEnd`), same as the TypeDef field/method lists.
- Signatures: `PropertySignature` decodes the PropertySig blob (§II.23.2.5,
  marker 0x08, masked because HASTHIS 0x20 combines with it).
- Flags: `EventAttributes`, `PropertyAttributes`, `MethodSemanticsAttributes`
  (§II.23.1.4/14/12).

The brute-force suites run against a second pinned fixture, the
`Windows.Foundation.UniversalApiContract.winmd` from the
`Microsoft.Windows.SDK.Contracts` NuGet package (that package ships ~94
per-contract winmds — there is no merged `Windows.winmd` on NuGet; its
`Windows.WinMD` entry is a type-forwarder facade). The Win32 winmd contains
**zero** event/property rows — `TestWin32HasNoEventsOrProperties` asserts
this — so the addition is inert for the Win32/WDK projections.

## Comparison with microsoft/go-winmd

`microsoft/go-winmd` is the closest prior art. It was evaluated and not adopted:
it has no tagged releases, depends on `golang.org/x/tools` for codegen, and —
critically for these projections — lacks a custom-attribute **value** decoder,
Constant-table decoding, and `#-` stream handling. Its lazy-table model is also
the wrong shape for a consumer that scans 100% of rows. This reader borrows its
good practices (spec-referenced docs, typed enums, bounds-checked allocation)
without the dependency.
