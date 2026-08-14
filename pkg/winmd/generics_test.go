package winmd

import "testing"

// blobWith wraps raw signature bytes as a #Blob heap holding one blob at
// offset 0 (1-byte compressed length prefix).
func blobWith(sig []byte) BlobHeap {
	if len(sig) >= 0x80 {
		panic("test signature too long for a 1-byte length prefix")
	}
	return BlobHeap(append([]byte{byte(len(sig))}, sig...))
}

// fileWithNamedType builds a File with a single TypeRef so resolveTypeToken
// can name a generic type in a synthetic signature.
func fileWithNamedType(namespace, name string) *File {
	f := &File{}
	f.Tables.TypeRefs = []TypeRefRow{{Namespace: namespace, Name: name}}
	return f
}

// TestDecodeGenericInst decodes a GENERICINST signature: a field of type
// IVector`1<U4> — GENERICINST CLASS <TypeRef #1> 1 U4.
func TestDecodeGenericInst(t *testing.T) {
	f := fileWithNamedType("Windows.Foundation.Collections", "IVector`1")
	// TypeDefOrRef encoding of TypeRef row 1: (1<<2)|1 = 0x05.
	sig := []byte{0x06, byte(ElemGenericInst), byte(ElemClass), 0x05, 0x01, byte(ElemUInt32)}
	f.Blobs = blobWith(sig)

	ts, err := f.FieldSignature(0)
	if err != nil {
		t.Fatalf("FieldSignature: %v", err)
	}
	if ts.Kind != SigGenericInst {
		t.Fatalf("kind = %d, want SigGenericInst", ts.Kind)
	}
	if ts.Name != "IVector`1" {
		t.Errorf("name = %q, want IVector`1", ts.Name)
	}
	if len(ts.GenericArgs) != 1 || ts.GenericArgs[0].Primitive != ElemUInt32 {
		t.Errorf("args = %+v, want [U4]", ts.GenericArgs)
	}
}

// TestDecodeVarMVar decodes the generic parameters VAR 0 and MVAR 1.
func TestDecodeVarMVar(t *testing.T) {
	f := &File{Blobs: blobWith([]byte{0x06, byte(ElemVar), 0x00})}
	ts, err := f.FieldSignature(0)
	if err != nil {
		t.Fatalf("VAR: %v", err)
	}
	if ts.Kind != SigVar || ts.GenericIndex != 0 {
		t.Errorf("VAR = %+v, want SigVar index 0", ts)
	}

	f = &File{Blobs: blobWith([]byte{0x06, byte(ElemMVar), 0x01})}
	ts, err = f.FieldSignature(0)
	if err != nil {
		t.Fatalf("MVAR: %v", err)
	}
	if ts.Kind != SigMVar || ts.GenericIndex != 1 {
		t.Errorf("MVAR = %+v, want SigMVar index 1", ts)
	}
}

// TestWin32HasNoGenerics is the tripwire that keeps go-bindings-win32 and
// go-bindings-wdk safe: the committed Win32 winmd must contain zero generic
// constructs, so adding generics decoding cannot change their output. If a
// future winmd introduces generics, this fails loudly.
func TestWin32HasNoGenerics(t *testing.T) {
	file := testFile(t)

	var hasGeneric func(ts *TypeSig) bool
	hasGeneric = func(ts *TypeSig) bool {
		switch ts.Kind {
		case SigGenericInst, SigVar, SigMVar:
			return true
		}
		if ts.Child != nil && hasGeneric(ts.Child) {
			return true
		}
		for i := range ts.GenericArgs {
			if hasGeneric(&ts.GenericArgs[i]) {
				return true
			}
		}
		return false
	}

	for i := range file.Tables.Methods {
		sig, err := file.MethodSignature(file.Tables.Methods[i].Signature)
		if err != nil {
			continue
		}
		if hasGeneric(&sig.Return) {
			t.Fatalf("method %s return is generic — win32 no longer generics-free", file.Tables.Methods[i].Name)
		}
		for j := range sig.Params {
			if hasGeneric(&sig.Params[j]) {
				t.Fatalf("method %s has a generic param", file.Tables.Methods[i].Name)
			}
		}
	}
	if len(file.Tables.GenericParams) != 0 {
		t.Fatalf("win32 winmd has %d GenericParam rows, want 0", len(file.Tables.GenericParams))
	}
}
