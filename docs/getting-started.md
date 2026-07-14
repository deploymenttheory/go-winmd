# Getting started

`go-winmd` reads ECMA-335 `.winmd` metadata files. It is stdlib-only.

```sh
go get github.com/deploymenttheory/go-winmd
```

## Open a file

```go
file, err := winmd.Open("Windows.Win32.winmd")
if err != nil {
	log.Fatal(err)
}
```

`file.Tables` holds the materialized tables as eager typed slices, and
`file.Strings` / `file.Blobs` / `file.GUIDs` are the heaps.

## Iterate types

```go
for i := range file.Tables.TypeDefs {
	td := &file.Tables.TypeDefs[i]
	switch {
	case td.Flags&winmd.TypeAttrInterface != 0:
		fmt.Println(td.Namespace, td.Name, "(COM interface)")
	case td.Flags&winmd.TypeAttrExplicitLayout != 0:
		fmt.Println(td.Namespace, td.Name, "(union / explicit layout)")
	}
}
```

Flag columns are typed (`TypeAttributes`, `FieldAttributes`, `ParamAttributes`,
`PInvokeAttributes`, …) with spec member-name constants and `String()` methods.

## Decode a method signature

Blob columns keep their `#Blob` offset; decode on demand:

```go
m := &file.Tables.Methods[0]
sig, err := file.MethodSignature(m.Signature)
if err != nil {
	log.Fatal(err)
}
fmt.Println("returns", sig.Return.Kind, "with", len(sig.Params), "params")
```

`sig.Return` / `sig.Params[i]` are `TypeSig` values — a recursive grammar
(`SigPrimitive`, `SigNamed`, `SigPointer`, `SigArray`, `SigSZArray`,
`SigFuncPtr`).

## Read custom-attribute values

Unlike most winmd readers, attribute values are decoded (not raw blobs):

```go
target := winmd.CodedIndex{Table: winmd.TableTypeDef, Row: uint32(i + 1)}
for _, attr := range file.AttributesFor(target) {
	if attr.Name == "GuidAttribute" {
		// attr.Fixed holds the 11 decoded GUID fields
	}
}
```

## Coded indices

A `CodedIndex` is a resolved `{Table, Row}` pair (1-based row; `Table ==
TableNull` when null). Switch on `.Table` to interpret a target:

```go
switch ref.Table {
case winmd.TableTypeDef:
	def := &file.Tables.TypeDefs[ref.Row-1]
	_ = def
case winmd.TableTypeRef:
	r := &file.Tables.TypeRefs[ref.Row-1]
	_ = r
}
```

See [ecma335-notes.md](ecma335-notes.md) for what is materialized versus
sized-only, and the intentional non-goals.
