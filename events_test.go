package winmd

import "testing"

// TestDecodePropertySig decodes a synthetic PropertySig: an instance
// property of type I4 — PROPERTY|HASTHIS (0x28), 0 params, I4.
func TestDecodePropertySig(t *testing.T) {
	f := &File{Blobs: blobWith([]byte{0x28, 0x00, byte(ElemInt32)})}
	sig, err := f.PropertySignature(0)
	if err != nil {
		t.Fatalf("PropertySignature: %v", err)
	}
	if !sig.HasThis {
		t.Error("HasThis not set for 0x28 marker")
	}
	if sig.Return.Kind != SigPrimitive || sig.Return.Primitive != ElemInt32 {
		t.Errorf("property type = %+v, want I4", sig.Return)
	}
	if len(sig.Params) != 0 {
		t.Errorf("params = %d, want 0", len(sig.Params))
	}
}

// TestDecodePropertySigIndexer decodes an indexer PropertySig with one I4
// parameter and an HSTRING (ELEMENT_TYPE_STRING) property type.
func TestDecodePropertySigIndexer(t *testing.T) {
	f := &File{Blobs: blobWith([]byte{0x28, 0x01, byte(ElemString), byte(ElemInt32)})}
	sig, err := f.PropertySignature(0)
	if err != nil {
		t.Fatalf("PropertySignature: %v", err)
	}
	if sig.Return.Primitive != ElemString {
		t.Errorf("property type = %+v, want STRING", sig.Return)
	}
	if len(sig.Params) != 1 || sig.Params[0].Primitive != ElemInt32 {
		t.Errorf("params = %+v, want [I4]", sig.Params)
	}
}

// TestDecodePropertySigBadMarker rejects a blob whose low nibble is not
// PROPERTY (0x08) — here a plain MethodDefSig marker.
func TestDecodePropertySigBadMarker(t *testing.T) {
	f := &File{Blobs: blobWith([]byte{0x20, 0x00, byte(ElemInt32)})}
	if _, err := f.PropertySignature(0); err == nil {
		t.Fatal("PropertySignature accepted a non-PROPERTY marker")
	}
}

// TestWin32HasNoEventsOrProperties is the tripwire that keeps
// go-bindings-win32 and go-bindings-wdk safe: the committed Win32 winmd must
// contain zero event/property constructs, so materializing those tables
// cannot change their output. If a future winmd introduces them, this fails
// loudly. (The sibling for generics is TestWin32HasNoGenerics.)
func TestWin32HasNoEventsOrProperties(t *testing.T) {
	file := testFile(t)
	tables := &file.Tables

	if n := len(tables.Events); n != 0 {
		t.Errorf("win32 winmd has %d Event rows, want 0", n)
	}
	if n := len(tables.EventMaps); n != 0 {
		t.Errorf("win32 winmd has %d EventMap rows, want 0", n)
	}
	if n := len(tables.Properties); n != 0 {
		t.Errorf("win32 winmd has %d Property rows, want 0", n)
	}
	if n := len(tables.PropertyMaps); n != 0 {
		t.Errorf("win32 winmd has %d PropertyMap rows, want 0", n)
	}
	if n := len(tables.MethodSemantics); n != 0 {
		t.Errorf("win32 winmd has %d MethodSemantics rows, want 0", n)
	}
}
