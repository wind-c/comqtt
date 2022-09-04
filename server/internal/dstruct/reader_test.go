package dynamicstruct

import (
	"reflect"
	"testing"
	"time"
)

type (
	testStructOne struct {
		String          string
		Integer         int
		Uinteger        uint
		Float           float64
		Bool            bool
		Time            time.Time
		PointerString   *string
		PointerInteger  *int
		PointerUinteger *uint
		PointerFloat    *float64
		PointerBool     *bool
		PointerTime     *time.Time
		Integers        []int
	}

	testStructTwo struct{}
)

func TestReaderImpl_GetField(t *testing.T) {
	reader := NewReader(testStructOne{
		String: "some text",
	})

	if _, ok := reader.GetField("String").(fieldImpl); !ok {
		t.Error(`TestReaderImpl_GetField - expected to have field "String"`)
	}
	if _, ok := reader.GetField("Unknown").(fieldImpl); ok {
		t.Error(`TestReaderImpl_GetField - expected not to have field "Unknown"`)
	}
}

func TestReaderImpl_HasField(t *testing.T) {
	reader := NewReader(testStructOne{
		String: "some text",
	})

	if !reader.HasField("String") {
		t.Error(`TestReaderImpl_HasField - expected to have field "String"`)
	}
	if reader.HasField("Unknown") {
		t.Error(`TestReaderImpl_HasField - expected not to have field "Unknown"`)
	}
}

func TestReaderImpl_GetAllFields(t *testing.T) {
	reader := NewReader(testStructOne{})

	if len(reader.GetAllFields()) != 13 {
		t.Errorf(`TestReaderImpl_GetAllFields - expected to have 13 fields but got %d`, len(reader.GetAllFields()))
	}
}

func TestReadImpl_HaveSameTypes(t *testing.T) {
	reader := readImpl{}

	integer := 0
	str := ""
	float := 0.0
	boolean := false

	testCases := []struct {
		first  interface{}
		second interface{}
		result bool
	}{
		{0, 1, true},
		{"foo", "bar", true},
		{0.0, 1.0, true},
		{true, false, true},
		{[]int{1}, []int{}, true},
		{&integer, &integer, true},
		{&str, &str, true},
		{&float, &float, true},
		{&boolean, &boolean, true},
		{testStructOne{}, testStructOne{}, true},
		{&testStructOne{}, &testStructOne{}, true},
		{map[string]int{}, map[string]int{}, true},
		{0, 1.0, false},
		{0, "foo", false},
		{0, false, false},
		{0, []int{}, false},
		{0, map[string]int{}, false},
		{[]float64{}, []int{}, false},
		{map[string]float64{}, map[string]int{}, false},
		{map[byte]int{}, map[string]int{}, false},
		{testStructOne{}, &testStructOne{}, false},
		{testStructOne{}, testStructTwo{}, false},
		{testStructOne{}, time.Time{}, false},
	}

	for _, testCase := range testCases {
		if reader.haveSameTypes(reflect.TypeOf(testCase.first), reflect.TypeOf(testCase.second)) != testCase.result {
			if testCase.result {
				t.Errorf(`TestReadImpl_HaveSameTypes - expected for %#v and %#v to have same type`, testCase.first, testCase.second)
			} else {
				t.Errorf(`TestReadImpl_HaveSameTypes - expected for %#v and %#v not to have same type`, testCase.first, testCase.second)
			}
		}
	}
}

func TestReadImpl_ToStruct(t *testing.T) {
	integer := 123
	uinteger := uint(456)
	str := "text"
	float := 123.45
	boolean := true
	now := time.Now()

	value := testStructOne{
		String:          str,
		Integer:         integer,
		Uinteger:        uinteger,
		Float:           float,
		Bool:            boolean,
		Time:            now,
		PointerString:   &str,
		PointerInteger:  &integer,
		PointerUinteger: &uinteger,
		PointerFloat:    &float,
		PointerBool:     &boolean,
		PointerTime:     &now,
		Integers:        []int{1, 2, 3},
	}

	reader := NewReader(value)

	var result testStructOne
	err := reader.ToStruct(&result)

	if err != nil {
		t.Errorf(`TestReadImpl_ToStruct - expected not to have error got %#v`, err)
	}

	if !reflect.DeepEqual(value, result) {
		t.Errorf(`TestReadImpl_ToStruct - expected struct to be equal %#v but got %#v`, result, value)
	}
}

func TestReadImpl_ToSliceOfReaders(t *testing.T) {
	integer := 123
	uinteger := uint(456)
	str := "text"
	float := 123.45
	boolean := true
	now := time.Now()

	value := []testStructOne{
		{
			String:          str,
			Integer:         integer,
			Uinteger:        uinteger,
			Float:           float,
			Bool:            boolean,
			Time:            now,
			PointerString:   &str,
			PointerInteger:  &integer,
			PointerUinteger: &uinteger,
			PointerFloat:    &float,
			PointerBool:     &boolean,
			PointerTime:     &now,
			Integers:        []int{1, 2, 3},
		},
	}

	reader := NewReader(value)

	result := reader.ToSliceOfReaders()

	if result == nil {
		t.Errorf(`TestReadImpl_ToSliceOfReaders - expected to have slice []Reader got %#v`, result)
	}

	if len(result) != 1 {
		t.Errorf(`TestReadImpl_ToSliceOfReaders - expected to have one element in slice []Reader got %#v`, len(result))
	}

	var parsed testStructOne
	err := result[0].ToStruct(&parsed)

	if err != nil {
		t.Errorf(`TestReadImpl_ToSliceOfReaders - expected not to have error got %#v`, err)
	}

	if !reflect.DeepEqual(value[0], parsed) {
		t.Errorf(`TestReadImpl_ToSliceOfReaders - expected slice of structs to be equal %#v but got %#v`, value[0], parsed)
	}
}

func TestReadImpl_ToMapReaderOfReaders(t *testing.T) {
	integer := 123
	uinteger := uint(456)
	str := "text"
	float := 123.45
	boolean := true
	now := time.Now()

	value := map[string]testStructOne{
		"element": {
			String:          str,
			Integer:         integer,
			Uinteger:        uinteger,
			Float:           float,
			Bool:            boolean,
			Time:            now,
			PointerString:   &str,
			PointerInteger:  &integer,
			PointerUinteger: &uinteger,
			PointerFloat:    &float,
			PointerBool:     &boolean,
			PointerTime:     &now,
			Integers:        []int{1, 2, 3},
		},
	}

	reader := NewReader(value)

	result := reader.ToMapReaderOfReaders()

	if result == nil {
		t.Errorf(`TestReadImpl_ToMapReaderOfReaders - expected to have slice []Reader got %#v`, result)
	}

	if len(result) != 1 {
		t.Errorf(`TestReadImpl_ToMapReaderOfReaders - expected to have one element in slice []Reader got %#v`, len(result))
	}

	var parsed testStructOne
	err := result["element"].ToStruct(&parsed)

	if err != nil {
		t.Errorf(`TestReadImpl_ToMapReaderOfReaders - expected not to have error got %#v`, err)
	}

	if !reflect.DeepEqual(value["element"], parsed) {
		t.Errorf(`TestReadImpl_ToMapReaderOfReaders - expected slice of structs to be equal %#v but got %#v`, value["element"], parsed)
	}
}

func TestFieldImpl_Name(t *testing.T) {
	reader := NewReader(testStructOne{})

	if reader.GetField("String").Name() != "String" {
		t.Errorf(`TestFieldImpl_Name - expected field name to be "String"  got %s`, reader.GetField("String").Name())
	}
}

func TestFieldImpl_PointerInt(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		PointerInteger: &expected,
	})

	value := reader.GetField("PointerInteger").PointerInt()

	if *value != expected {
		t.Errorf(`TestFieldImpl_PointerInt - expected field "PointerInteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerInteger").PointerInt()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerInt - expected field "PointerInteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Int(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		Integer: expected,
	})

	value := reader.GetField("Integer").Int()

	if value != expected {
		t.Errorf(`TestFieldImpl_Int - expected field "Integer" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerInt8(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		PointerInteger: &expected,
	})

	value := reader.GetField("PointerInteger").PointerInt8()

	if *value != int8(expected) {
		t.Errorf(`TestFieldImpl_PointerInt8 - expected field "PointerInteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerInteger").PointerInt8()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerInt8 - expected field "PointerInteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Int8(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		Integer: expected,
	})

	value := reader.GetField("Integer").Int8()

	if value != int8(expected) {
		t.Errorf(`TestFieldImpl_Int8 - expected field "Integer" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerInt16(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		PointerInteger: &expected,
	})

	value := reader.GetField("PointerInteger").PointerInt16()

	if *value != int16(expected) {
		t.Errorf(`TestFieldImpl_PointerInt16 - expected field "PointerInteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerInteger").PointerInt16()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerInt16 - expected field "PointerInteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Int16(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		Integer: expected,
	})

	value := reader.GetField("Integer").Int16()

	if value != int16(expected) {
		t.Errorf(`TestFieldImpl_Int16 - expected field "Integer" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerInt32(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		PointerInteger: &expected,
	})

	value := reader.GetField("PointerInteger").PointerInt32()

	if *value != int32(expected) {
		t.Errorf(`TestFieldImpl_PointerInt32 - expected field "PointerInteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerInteger").PointerInt32()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerInt32 - expected field "PointerInteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Int32(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		Integer: expected,
	})

	value := reader.GetField("Integer").Int32()

	if value != int32(expected) {
		t.Errorf(`TestFieldImpl_Int32 - expected field "Integer" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerInt64(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		PointerInteger: &expected,
	})

	value := reader.GetField("PointerInteger").PointerInt64()

	if *value != int64(expected) {
		t.Errorf(`TestFieldImpl_PointerInt64 - expected field "PointerInteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerInteger").PointerInt64()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerInt64 - expected field "PointerInteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Int64(t *testing.T) {
	expected := 123

	reader := NewReader(testStructOne{
		Integer: expected,
	})

	value := reader.GetField("Integer").Int64()

	if value != int64(expected) {
		t.Errorf(`TestFieldImpl_Int64 - expected field "Integer" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerUint(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		PointerUinteger: &expected,
	})

	value := reader.GetField("PointerUinteger").PointerUint()

	if *value != expected {
		t.Errorf(`TestFieldImpl_PointerUint - expected field "PointerUinteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerUinteger").PointerUint()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerUint - expected field "PointerUinteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Uint(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		Uinteger: expected,
	})

	value := reader.GetField("Uinteger").Uint()

	if value != expected {
		t.Errorf(`TestFieldImpl_Uint - expected field "Uinteger" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerUint8(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		PointerUinteger: &expected,
	})

	value := reader.GetField("PointerUinteger").PointerUint8()

	if *value != uint8(expected) {
		t.Errorf(`TestFieldImpl_PointerUint8 - expected field "PointerUinteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerUinteger").PointerUint8()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerUint8 - expected field "PointerUinteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Uint8(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		Uinteger: expected,
	})

	value := reader.GetField("Uinteger").Uint8()

	if value != uint8(expected) {
		t.Errorf(`TestFieldImpl_Uint8 - expected field "Uinteger" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerUint16(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		PointerUinteger: &expected,
	})

	value := reader.GetField("PointerUinteger").PointerUint16()

	if *value != uint16(expected) {
		t.Errorf(`TestFieldImpl_PointerUint16 - expected field "PointerUinteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerUinteger").PointerUint16()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerUint16 - expected field "PointerUinteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Uint16(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		Uinteger: expected,
	})

	value := reader.GetField("Uinteger").Uint16()

	if value != uint16(expected) {
		t.Errorf(`TestFieldImpl_Uint16 - expected field "Uinteger" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerUint32(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		PointerUinteger: &expected,
	})

	value := reader.GetField("PointerUinteger").PointerUint32()

	if *value != uint32(expected) {
		t.Errorf(`TestFieldImpl_PointerUint32 - expected field "PointerUinteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerUinteger").PointerUint32()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerUint32 - expected field "PointerUinteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Uint32(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		Uinteger: expected,
	})

	value := reader.GetField("Uinteger").Uint32()

	if value != uint32(expected) {
		t.Errorf(`TestFieldImpl_Uint32 - expected field "Uinteger" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerUint64(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		PointerUinteger: &expected,
	})

	value := reader.GetField("PointerUinteger").PointerUint64()

	if *value != uint64(expected) {
		t.Errorf(`TestFieldImpl_PointerUint64 - expected field "PointerUinteger" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerUinteger").PointerUint64()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerUint64 - expected field "PointerUinteger" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Uint64(t *testing.T) {
	expected := uint(123)

	reader := NewReader(testStructOne{
		Uinteger: expected,
	})

	value := reader.GetField("Uinteger").Uint64()

	if value != uint64(expected) {
		t.Errorf(`TestFieldImpl_Uint64 - expected field "Uinteger" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerFloat32(t *testing.T) {
	expected := 123.0

	reader := NewReader(testStructOne{
		PointerFloat: &expected,
	})

	value := reader.GetField("PointerFloat").PointerFloat32()

	if *value != float32(expected) {
		t.Errorf(`TestFieldImpl_PointerFloat32 - expected field "PointerFloat" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerFloat").PointerFloat32()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerFloat32 - expected field "PointerFloat" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Float32(t *testing.T) {
	expected := 123.0

	reader := NewReader(testStructOne{
		Float: expected,
	})

	value := reader.GetField("Float").Float32()

	if value != float32(expected) {
		t.Errorf(`TestFieldImpl_Float32 - expected field "Float" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerFloat64(t *testing.T) {
	expected := 123.0

	reader := NewReader(testStructOne{
		PointerFloat: &expected,
	})

	value := reader.GetField("PointerFloat").PointerFloat64()

	if *value != expected {
		t.Errorf(`TestFieldImpl_PointerFloat64 - expected field "PointerFloat" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerFloat").PointerFloat64()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerFloat64 - expected field "PointerFloat" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Float64(t *testing.T) {
	expected := 123.0

	reader := NewReader(testStructOne{
		Float: expected,
	})

	value := reader.GetField("Float").Float64()

	if value != expected {
		t.Errorf(`TestFieldImpl_Float64 - expected field "Float" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerString(t *testing.T) {
	expected := "something"

	reader := NewReader(testStructOne{
		PointerString: &expected,
	})

	value := reader.GetField("PointerString").PointerString()

	if *value != expected {
		t.Errorf(`TestFieldImpl_PointerString - expected field "PointerString" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerString").PointerString()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerString - expected field "PointerString" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_String(t *testing.T) {
	expected := "something"

	reader := NewReader(testStructOne{
		String: expected,
	})

	value := reader.GetField("String").String()

	if value != expected {
		t.Errorf(`TestFieldImpl_String - expected field "String" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerBool(t *testing.T) {
	expected := true

	reader := NewReader(testStructOne{
		PointerBool: &expected,
	})

	value := reader.GetField("PointerBool").PointerBool()

	if *value != expected {
		t.Errorf(`TestFieldImpl_PointerBool - expected field "PointerBool" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerBool").PointerBool()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerBool - expected field "PointerBool" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Bool(t *testing.T) {
	expected := true

	reader := NewReader(testStructOne{
		Bool: expected,
	})

	value := reader.GetField("Bool").Bool()

	if value != expected {
		t.Errorf(`TestFieldImpl_Bool - expected field "Bool" to be equal %#v but got %#v`, expected, value)
	}
}

func TestFieldImpl_PointerTime(t *testing.T) {
	expected := time.Now()

	reader := NewReader(testStructOne{
		PointerTime: &expected,
	})

	value := reader.GetField("PointerTime").PointerTime()

	if *value != expected {
		t.Errorf(`TestFieldImpl_PointerTime - expected field "PointerTime" to be equal %#v but got %#v`, expected, *value)
	}

	reader = NewReader(testStructOne{})

	value = reader.GetField("PointerTime").PointerTime()

	if value != nil {
		t.Errorf(`TestFieldImpl_PointerTime - expected field "PointerTime" to be nil but got %#v`, *value)
	}
}

func TestFieldImpl_Time(t *testing.T) {
	expected := time.Now()

	reader := NewReader(testStructOne{
		Time: expected,
	})

	value := reader.GetField("Time").Time()

	if value != expected {
		t.Errorf(`TestFieldImpl_Time - expected field "Time" to be equal %#v but got %#v`, expected, value)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("TestFieldImpl_Time - expected panic by casting int to instance of time.Time{}")
		}
	}()
	value = reader.GetField("Integer").Time()
}

func TestFieldImpl_Interface(t *testing.T) {
	expected := []int{1, 2, 3}

	reader := NewReader(testStructOne{
		Integers: expected,
	})

	value, ok := reader.GetField("Integers").Interface().([]int)

	if !ok {
		t.Error(`TestFieldImpl_Interface - expected field "String" to be instance of []int`)
	}

	if !reflect.DeepEqual(value, expected) {
		t.Errorf(`TestFieldImpl_Interface - expected field "String" to be equal %#v but got %#v`, expected, value)
	}
}
