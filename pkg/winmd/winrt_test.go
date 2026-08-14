package winmd

import (
	"fmt"
	"strings"
	"testing"
)

// The WinRT brute-force suites run against the pinned
// Windows.Foundation.UniversalApiContract.winmd (see fixture_test.go). They
// hold the same zero-failure bar as the Win32 suites: any construct the
// decoder does not understand fails the test.

// TestWinRTDecodeAllSignatures brute-forces every method, field, and
// TypeSpec signature in the WinRT winmd — generics at real scale.
func TestWinRTDecodeAllSignatures(t *testing.T) {
	file := testWinRTFile(t)

	methodFailures := 0
	for i := range file.Tables.Methods {
		method := &file.Tables.Methods[i]
		if _, err := file.MethodSignature(method.Signature); err != nil {
			methodFailures++
			if methodFailures <= 5 {
				t.Errorf("method %s: %v", method.Name, err)
			}
		}
	}
	fieldFailures := 0
	for i := range file.Tables.Fields {
		field := &file.Tables.Fields[i]
		if _, err := file.FieldSignature(field.Signature); err != nil {
			fieldFailures++
			if fieldFailures <= 5 {
				t.Errorf("field %s: %v", field.Name, err)
			}
		}
	}
	typeSpecFailures := 0
	for i, blobOffset := range file.Tables.TypeSpecs {
		if _, err := file.TypeSpecSignature(blobOffset); err != nil {
			typeSpecFailures++
			if typeSpecFailures <= 5 {
				t.Errorf("typespec row %d: %v", i+1, err)
			}
		}
	}
	t.Logf("decoded %d method, %d field, %d typespec sigs (%d/%d/%d failures)",
		len(file.Tables.Methods), len(file.Tables.Fields), len(file.Tables.TypeSpecs),
		methodFailures, fieldFailures, typeSpecFailures)
	if methodFailures > 0 || fieldFailures > 0 || typeSpecFailures > 0 {
		t.Fatalf("%d method + %d field + %d typespec signature failures",
			methodFailures, fieldFailures, typeSpecFailures)
	}
}

// TestWinRTDecodeAllPropertySignatures brute-forces every PropertySig blob
// and checks that each property's HASTHIS agrees with its getter: instance
// accessors (interface members) carry it, static accessors (class-level
// statics) do not.
func TestWinRTDecodeAllPropertySignatures(t *testing.T) {
	file := testWinRTFile(t)
	tables := &file.Tables

	if len(tables.Properties) == 0 {
		t.Fatal("WinRT winmd has no Property rows — materialization broken")
	}
	// Property row → getter MethodDef row.
	getters := make(map[uint32]uint32, len(tables.Properties))
	for i := range tables.MethodSemantics {
		row := &tables.MethodSemantics[i]
		if row.Association.Table == TableProperty && row.Semantics&MethodSemanticsGetter != 0 {
			getters[row.Association.Row] = row.Method
		}
	}

	failures := 0
	mismatches := 0
	instance, static := 0, 0
	for i := range tables.Properties {
		property := &tables.Properties[i]
		sig, err := file.PropertySignature(property.Type)
		if err != nil {
			failures++
			if failures <= 5 {
				t.Errorf("property %s: %v", property.Name, err)
			}
			continue
		}
		if sig.HasThis {
			instance++
		} else {
			static++
		}
		if getter, ok := getters[uint32(i+1)]; ok {
			getterStatic := tables.Methods[getter-1].Flags&MethodAttrStatic != 0
			if sig.HasThis == getterStatic {
				mismatches++
				if mismatches <= 5 {
					t.Errorf("property %s: HasThis=%v but getter static=%v", property.Name, sig.HasThis, getterStatic)
				}
			}
		}
	}
	t.Logf("decoded %d property sigs (%d failures): %d instance, %d static, %d this/static mismatches",
		len(tables.Properties), failures, instance, static, mismatches)
	if failures > 0 || mismatches > 0 {
		t.Fatalf("%d property signature failures, %d this/static mismatches", failures, mismatches)
	}
}

// TestWinRTDecodeAllAttributes brute-forces every custom attribute blob and
// asserts the WinRT class-model attributes all decode.
func TestWinRTDecodeAllAttributes(t *testing.T) {
	file := testWinRTFile(t)

	histogram := map[string]int{}
	failures := 0
	for row := range file.Tables.CustomAttributes {
		attr, err := file.decodeAttribute(&file.Tables.CustomAttributes[row])
		if err != nil {
			failures++
			if failures <= 5 {
				t.Logf("row %d: %v", row, err)
			}
			continue
		}
		histogram[attr.Name]++
	}
	total := len(file.Tables.CustomAttributes)
	t.Logf("decoded %d/%d attributes; distinct names: %d", total-failures, total, len(histogram))
	for _, name := range []string{
		"GuidAttribute", "ActivatableAttribute", "StaticAttribute",
		"ComposableAttribute", "ExclusiveToAttribute", "DefaultAttribute",
		"OverloadAttribute", "DefaultOverloadAttribute", "FlagsAttribute",
		"ContractVersionAttribute", "DeprecatedAttribute",
	} {
		if histogram[name] == 0 {
			t.Errorf("attribute %s: decoded 0 instances", name)
		}
	}
	if failures > 0 {
		t.Errorf("%d attribute blobs failed to decode", failures)
	}
}

// TestWinRTEventPropertySemantics checks the structural integrity of the
// event/property tables: every map parent and list range is in bounds, every
// MethodSemantics row resolves, and the accessor naming contract holds
// (get_/put_ for properties, add_/remove_ for events).
func TestWinRTEventPropertySemantics(t *testing.T) {
	file := testWinRTFile(t)
	tables := &file.Tables

	if len(tables.EventMaps) == 0 || len(tables.PropertyMaps) == 0 || len(tables.MethodSemantics) == 0 {
		t.Fatalf("WinRT winmd: %d EventMaps, %d PropertyMaps, %d MethodSemantics — materialization broken",
			len(tables.EventMaps), len(tables.PropertyMaps), len(tables.MethodSemantics))
	}

	for i := range tables.EventMaps {
		eventMap := &tables.EventMaps[i]
		if eventMap.Parent == 0 || int(eventMap.Parent) > len(tables.TypeDefs) {
			t.Fatalf("EventMap %d: parent TypeDef row %d out of range", i, eventMap.Parent)
		}
		if eventMap.EventFirst > eventMap.EventEnd || int(eventMap.EventEnd) > len(tables.Events)+1 {
			t.Fatalf("EventMap %d: range [%d,%d) out of bounds (%d events)",
				i, eventMap.EventFirst, eventMap.EventEnd, len(tables.Events))
		}
	}
	for i := range tables.PropertyMaps {
		propertyMap := &tables.PropertyMaps[i]
		if propertyMap.Parent == 0 || int(propertyMap.Parent) > len(tables.TypeDefs) {
			t.Fatalf("PropertyMap %d: parent TypeDef row %d out of range", i, propertyMap.Parent)
		}
		if propertyMap.PropertyFirst > propertyMap.PropertyEnd || int(propertyMap.PropertyEnd) > len(tables.Properties)+1 {
			t.Fatalf("PropertyMap %d: range [%d,%d) out of bounds (%d properties)",
				i, propertyMap.PropertyFirst, propertyMap.PropertyEnd, len(tables.Properties))
		}
	}

	// Group accessor semantics by their owning event/property.
	semantics := map[uint64][]int{} // attrTarget(Association) → MethodSemantics rows
	for i := range tables.MethodSemantics {
		row := &tables.MethodSemantics[i]
		if row.Method == 0 || int(row.Method) > len(tables.Methods) {
			t.Fatalf("MethodSemantics %d: MethodDef row %d out of range", i, row.Method)
		}
		switch row.Association.Table {
		case TableEvent:
			if int(row.Association.Row) > len(tables.Events) {
				t.Fatalf("MethodSemantics %d: Event row %d out of range", i, row.Association.Row)
			}
		case TableProperty:
			if int(row.Association.Row) > len(tables.Properties) {
				t.Fatalf("MethodSemantics %d: Property row %d out of range", i, row.Association.Row)
			}
		default:
			t.Fatalf("MethodSemantics %d: association table %v, want Event or Property", i, row.Association.Table)
		}
		semantics[attrTarget(row.Association)] = append(semantics[attrTarget(row.Association)], i)
	}

	// Every event must have add_<Name> and remove_<Name> accessors.
	for i := range tables.Events {
		event := &tables.Events[i]
		key := attrTarget(CodedIndex{Table: TableEvent, Row: uint32(i + 1)})
		var hasAdd, hasRemove bool
		for _, semRow := range semantics[key] {
			row := &tables.MethodSemantics[semRow]
			name := tables.Methods[row.Method-1].Name
			if row.Semantics&MethodSemanticsAddOn != 0 {
				hasAdd = true
				if name != "add_"+event.Name {
					t.Fatalf("event %s: AddOn method named %s", event.Name, name)
				}
			}
			if row.Semantics&MethodSemanticsRemoveOn != 0 {
				hasRemove = true
				if name != "remove_"+event.Name {
					t.Fatalf("event %s: RemoveOn method named %s", event.Name, name)
				}
			}
		}
		if !hasAdd || !hasRemove {
			t.Fatalf("event %s: missing add/remove accessors (add=%v remove=%v)", event.Name, hasAdd, hasRemove)
		}
	}

	// Every property getter must be named get_<Name>.
	for i := range tables.Properties {
		property := &tables.Properties[i]
		key := attrTarget(CodedIndex{Table: TableProperty, Row: uint32(i + 1)})
		for _, semRow := range semantics[key] {
			row := &tables.MethodSemantics[semRow]
			name := tables.Methods[row.Method-1].Name
			if row.Semantics&MethodSemanticsGetter != 0 && name != "get_"+property.Name {
				t.Fatalf("property %s: Getter method named %s", property.Name, name)
			}
			if row.Semantics&MethodSemanticsSetter != 0 && name != "put_"+property.Name {
				t.Fatalf("property %s: Setter method named %s", property.Name, name)
			}
		}
	}
	t.Logf("%d events, %d properties, %d semantics rows verified",
		len(tables.Events), len(tables.Properties), len(tables.MethodSemantics))
}

// TestWinRTCalendarSpotCheck pins the golden facts the go-bindings-winrt
// Calendar vertical builds on: the runtime class exists with the
// WindowsRuntime flag, ICalendar's first method is Clone (vtable slot 6),
// and its Year property has both accessors.
func TestWinRTCalendarSpotCheck(t *testing.T) {
	file := testWinRTFile(t)
	tables := &file.Tables

	findTypeDef := func(namespace, name string) (uint32, *TypeDefRow) {
		for i := range tables.TypeDefs {
			typeDef := &tables.TypeDefs[i]
			if typeDef.Namespace == namespace && typeDef.Name == name {
				return uint32(i + 1), typeDef
			}
		}
		t.Fatalf("%s.%s not found", namespace, name)
		return 0, nil
	}

	_, calendar := findTypeDef("Windows.Globalization", "Calendar")
	if calendar.Flags&TypeAttrWindowsRuntime == 0 {
		t.Error("Calendar TypeDef lacks the WindowsRuntime flag")
	}

	icalendarRow, icalendar := findTypeDef("Windows.Globalization", "ICalendar")
	if first := tables.Methods[icalendar.MethodFirst-1].Name; first != "Clone" {
		t.Errorf("ICalendar first method = %s, want Clone (pins vtable slot 6)", first)
	}

	// Year property: present with get + put semantics.
	var yearProperty uint32
	for i := range tables.PropertyMaps {
		propertyMap := &tables.PropertyMaps[i]
		if propertyMap.Parent != icalendarRow {
			continue
		}
		for row := propertyMap.PropertyFirst; row < propertyMap.PropertyEnd; row++ {
			if tables.Properties[row-1].Name == "Year" {
				yearProperty = row
			}
		}
	}
	if yearProperty == 0 {
		t.Fatal("ICalendar has no Year property")
	}
	var hasGet, hasPut bool
	for i := range tables.MethodSemantics {
		row := &tables.MethodSemantics[i]
		if row.Association.Table != TableProperty || row.Association.Row != yearProperty {
			continue
		}
		hasGet = hasGet || row.Semantics&MethodSemanticsGetter != 0
		hasPut = hasPut || row.Semantics&MethodSemanticsSetter != 0
	}
	if !hasGet || !hasPut {
		t.Errorf("Year property accessors: get=%v put=%v, want both", hasGet, hasPut)
	}

	// The version string identifies WinRT metadata (e.g. "WindowsRuntime 1.4").
	if !strings.HasPrefix(file.Version, "WindowsRuntime") {
		t.Errorf("metadata version = %q, want a WindowsRuntime version string", file.Version)
	}

	// GuidAttribute on ICalendar reassembles to the known IID.
	attrs := file.AttributesFor(CodedIndex{Table: TableTypeDef, Row: icalendarRow})
	for _, attr := range attrs {
		if attr.Name != "GuidAttribute" || len(attr.Fixed) != 11 {
			continue
		}
		guid := fmt.Sprintf("%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
			attr.Fixed[0], attr.Fixed[1], attr.Fixed[2],
			attr.Fixed[3], attr.Fixed[4], attr.Fixed[5], attr.Fixed[6],
			attr.Fixed[7], attr.Fixed[8], attr.Fixed[9], attr.Fixed[10])
		if guid != "CA30221D-86D9-40FB-A26B-D44EB7CF08EA" {
			t.Errorf("ICalendar IID = %s, want CA30221D-86D9-40FB-A26B-D44EB7CF08EA", guid)
		}
		return
	}
	t.Fatal("ICalendar has no GuidAttribute")
}
