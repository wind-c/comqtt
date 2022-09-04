package dynamicstruct

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

type (
	// Reader is helper interface which provides possibility to
	// access struct's fields' values, reads and provides theirs values.
	Reader interface {
		// HasField checks if struct instance has a field with a given name.
		//
		// if reader.HasField("SomeFloatField") { ...
		//
		HasField(name string) bool
		// GetField returns struct instance's field value.
		// If there is no such field, it returns nil.
		// Usable to edit existing struct's field.
		//
		// field := reader.GetField("SomeFloatField")
		//
		GetField(name string) Field
		// GetAllFields returns a list of all struct instance's fields.
		//
		// for _, field := range reader.GetAllFields() { ...
		//
		GetAllFields() []Field
		// ToStruct maps all read values to passed instance of struct, by setting
		// all its values for fields with same names.
		// It returns an error if argument is not a pointer to a struct.
		//
		// err := reader.ToStruct(&instance)
		//
		ToStruct(value interface{}) error
		// ToSliceOfReaders returns a list of Reader interfaces if value is representation
		// of slice itself.
		//
		// readers := reader.ToReaderSlice()
		//
		ToSliceOfReaders() []Reader
		// ToMapReaderOfReaders returns a map of Reader interfaces if value is representation
		// of map itself with some key type.
		//
		// readers := reader.ToReaderMap()
		//
		ToMapReaderOfReaders() map[interface{}]Reader
		// GetValue returns original value used in reader.
		//
		// instance := reader.GetValue()
		//
		GetValue() interface{}
	}

	// Field is a wrapper for struct's field's value.
	// It provides methods for easier field's value reading.
	Field interface {
		// GetName returns field's name in struct.
		//
		// name := field.GetName()
		//
		Name() string
		// PointerInt returns a pointer for instance of int type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").PointerInt()
		//
		PointerInt() *int
		// Int returns an instance of int type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").Int()
		//
		Int() int
		// PointerInt8 returns a pointer for instance of int8 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").PointerInt8()
		//
		PointerInt8() *int8
		// Int8 returns aan instance of int8 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").Int8()
		//
		Int8() int8
		// PointerInt16 returns a pointer for instance of int16 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").PointerInt16()
		//
		PointerInt16() *int16
		// Int16 returns an instance of int16 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").Int16()
		//
		Int16() int16
		// PointerInt32 returns a pointer for instance of int32 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").PointerInt32()
		//
		PointerInt32() *int32
		// Int32 returns an instance of int32 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").Int32()
		//
		Int32() int32
		// PointerInt64 returns a pointer for instance of int64 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").PointerInt64()
		//
		PointerInt64() *int64
		// Int64 returns an instance of int64 type.
		// It panics if field's value can't be casted to desired type.
		//
		// number := reader.GetField("SomeField").Int64()
		//
		Int64() int64
		// PointerUint returns a pointer for instance of uint type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").PointerUint()
		//
		PointerUint() *uint
		// Uint returns an instance of uint type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").Uint()
		//
		Uint() uint
		// PointerUint8 returns a pointer for instance of uint8 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").PointerUint8()
		//
		PointerUint8() *uint8
		// Uint8 returns an instance of uint8 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").Uint8()
		//
		Uint8() uint8
		// PointerUint16 returns a pointer for instance of uint16 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").PointerUint16()
		//
		PointerUint16() *uint16
		// Uint16 returns an instance of uint16 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").Uint16()
		//
		Uint16() uint16
		// PointerUint32 returns a pointer for instance of uint32 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").PointerUint32()
		//
		PointerUint32() *uint32
		// Uint32 returns an instance of uint32 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").Uint32()
		//
		Uint32() uint32
		// PointerUint64 returns a pointer for instance of uint64 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").PointerUint64()
		//
		PointerUint64() *uint64
		// Uint64 returns an instance of uint64 type.
		// It panics if field's value can't be casted to desired type.
		//
		// unsigned := reader.GetField("SomeField").Uint64()
		//
		Uint64() uint64
		// PointerFloat32 returns a pointer for instance of float32 type.
		// It panics if field's value can't be casted to desired type.
		//
		// boolean := reader.GetField("SomeField").PointerFloat32()
		//
		PointerFloat32() *float32
		// Float32 returns an of float32 type.
		// It panics if field's value can't be casted to desired type.
		//
		// boolean := reader.GetField("SomeField").Float32()
		//
		Float32() float32
		// PointerFloat64 returns a pointer for instance of float64 type.
		// It panics if field's value can't be casted to desired type.
		//
		// boolean := reader.GetField("SomeField").PointerFloat64()
		//
		PointerFloat64() *float64
		// Float64 returns an instance of float64 type.
		// It panics if field's value can't be casted to desired type.
		//
		// boolean := reader.GetField("SomeField").Float64()
		//
		Float64() float64
		// PointerString returns a pointer for instance of string type.
		// It panics if field's value can't be casted to desired type.
		//
		// text := reader.GetField("SomeField").PointerString()
		//
		PointerString() *string
		// String returns aan instance of string type.
		// It panics if field's value can't be casted to desired type.
		//
		// text := reader.GetField("SomeField").String()
		//
		String() string
		// PointerBool returns a pointer for instance of bool type.
		// It panics if field's value can't be casted to desired type.
		//
		// boolean := reader.GetField("SomeField").PointerBool()
		//
		PointerBool() *bool
		// Bool returns an instance of bool type.
		// It panics if field's value can't be casted to desired type.
		//
		// boolean := reader.GetField("SomeField").Bool()
		//
		Bool() bool
		// PointerTime returns a pointer for instance of time.Time{} type.
		// It panics if field's value can't be casted to desired type.
		//
		// dateTime := reader.GetField("SomeField").PointerTime()
		//
		PointerTime() *time.Time
		// Time returns an instance of time.Time{} type.
		// It panics if field's value can't be casted to desired type.
		//
		// dateTime := reader.GetField("SomeField").Time()
		//
		Time() time.Time
		// Interface returns an interface which represents field's value.
		// Useful for casting value into desired type.
		//
		// slice, ok := reader.GetField("SomeField").Interface().([]int)
		//
		Interface() interface{}
	}

	readImpl struct {
		fields map[string]fieldImpl
		value  interface{}
	}

	fieldImpl struct {
		field reflect.StructField
		value reflect.Value
	}
)

// NewReader reads struct instance and provides instance of
// Reader interface to give possibility to read all fields' values.
func NewReader(value interface{}) Reader {
	fields := map[string]fieldImpl{}

	valueOf := reflect.Indirect(reflect.ValueOf(value))
	typeOf := valueOf.Type()

	if typeOf.Kind() == reflect.Struct {
		for i := 0; i < valueOf.NumField(); i++ {
			field := typeOf.Field(i)
			fields[field.Name] = fieldImpl{
				field: field,
				value: valueOf.Field(i),
			}
		}
	}

	return readImpl{
		fields: fields,
		value:  value,
	}
}

func (r readImpl) HasField(name string) bool {
	_, ok := r.fields[name]
	return ok
}

func (r readImpl) GetField(name string) Field {
	if !r.HasField(name) {
		return nil
	}
	return r.fields[name]
}

func (r readImpl) GetAllFields() []Field {
	var fields []Field

	for _, field := range r.fields {
		fields = append(fields, field)
	}

	return fields
}

func (r readImpl) ToStruct(value interface{}) error {
	valueOf := reflect.ValueOf(value)

	if valueOf.Kind() != reflect.Ptr || valueOf.IsNil() {
		return errors.New("ToStruct: expected a pointer as an argument")
	}

	valueOf = valueOf.Elem()
	typeOf := valueOf.Type()

	if valueOf.Kind() != reflect.Struct {
		return errors.New("ToStruct: expected a pointer to struct as an argument")
	}

	for i := 0; i < valueOf.NumField(); i++ {
		fieldType := typeOf.Field(i)
		fieldValue := valueOf.Field(i)

		original, ok := r.fields[fieldType.Name]
		if !ok {
			continue
		}

		if fieldValue.CanSet() && r.haveSameTypes(original.value.Type(), fieldValue.Type()) {
			fieldValue.Set(original.value)
		}
	}

	return nil
}

func (r readImpl) ToSliceOfReaders() []Reader {
	valueOf := reflect.Indirect(reflect.ValueOf(r.value))
	typeOf := valueOf.Type()

	if typeOf.Kind() != reflect.Slice && typeOf.Kind() != reflect.Array {
		return nil
	}

	var readers []Reader

	for i := 0; i < valueOf.Len(); i++ {
		readers = append(readers, NewReader(valueOf.Index(i).Interface()))
	}

	return readers
}

func (r readImpl) ToMapReaderOfReaders() map[interface{}]Reader {
	valueOf := reflect.Indirect(reflect.ValueOf(r.value))
	typeOf := valueOf.Type()

	if typeOf.Kind() != reflect.Map {
		return nil
	}

	readers := map[interface{}]Reader{}

	for _, keyValue := range valueOf.MapKeys() {
		readers[keyValue.Interface()] = NewReader(valueOf.MapIndex(keyValue).Interface())
	}

	return readers
}

func (r readImpl) GetValue() interface{} {
	return r.value
}

func (r readImpl) haveSameTypes(first reflect.Type, second reflect.Type) bool {
	if first.Kind() != second.Kind() {
		return false
	}

	switch first.Kind() {
	case reflect.Ptr:
		return r.haveSameTypes(first.Elem(), second.Elem())
	case reflect.Struct:
		return first.PkgPath() == second.PkgPath() && first.Name() == second.Name()
	case reflect.Slice:
		return r.haveSameTypes(first.Elem(), second.Elem())
	case reflect.Map:
		return r.haveSameTypes(first.Elem(), second.Elem()) && r.haveSameTypes(first.Key(), second.Key())
	default:
		return first.Kind() == second.Kind()
	}
}

func (f fieldImpl) Name() string {
	return f.field.Name
}

func (f fieldImpl) PointerInt() *int {
	if f.value.IsNil() {
		return nil
	}
	value := f.Int()
	return &value
}

func (f fieldImpl) Int() int {
	return int(reflect.Indirect(f.value).Int())
}

func (f fieldImpl) PointerInt8() *int8 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Int8()
	return &value
}

func (f fieldImpl) Int8() int8 {
	return int8(reflect.Indirect(f.value).Int())
}

func (f fieldImpl) PointerInt16() *int16 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Int16()
	return &value
}

func (f fieldImpl) Int16() int16 {
	return int16(reflect.Indirect(f.value).Int())
}

func (f fieldImpl) PointerInt32() *int32 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Int32()
	return &value
}

func (f fieldImpl) Int32() int32 {
	return int32(reflect.Indirect(f.value).Int())
}

func (f fieldImpl) PointerInt64() *int64 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Int64()
	return &value
}

func (f fieldImpl) Int64() int64 {
	return reflect.Indirect(f.value).Int()
}

func (f fieldImpl) PointerUint() *uint {
	if f.value.IsNil() {
		return nil
	}
	value := f.Uint()
	return &value
}

func (f fieldImpl) Uint() uint {
	return uint(reflect.Indirect(f.value).Uint())
}

func (f fieldImpl) PointerUint8() *uint8 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Uint8()
	return &value
}

func (f fieldImpl) Uint8() uint8 {
	return uint8(reflect.Indirect(f.value).Uint())
}

func (f fieldImpl) PointerUint16() *uint16 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Uint16()
	return &value
}

func (f fieldImpl) Uint16() uint16 {
	return uint16(reflect.Indirect(f.value).Uint())
}

func (f fieldImpl) PointerUint32() *uint32 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Uint32()
	return &value
}

func (f fieldImpl) Uint32() uint32 {
	return uint32(reflect.Indirect(f.value).Uint())
}

func (f fieldImpl) PointerUint64() *uint64 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Uint64()
	return &value
}

func (f fieldImpl) Uint64() uint64 {
	return reflect.Indirect(f.value).Uint()
}

func (f fieldImpl) PointerFloat32() *float32 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Float32()
	return &value
}

func (f fieldImpl) Float32() float32 {
	return float32(reflect.Indirect(f.value).Float())
}

func (f fieldImpl) PointerFloat64() *float64 {
	if f.value.IsNil() {
		return nil
	}
	value := f.Float64()
	return &value
}

func (f fieldImpl) Float64() float64 {
	return reflect.Indirect(f.value).Float()
}

func (f fieldImpl) PointerString() *string {
	if f.value.IsNil() {
		return nil
	}
	value := f.String()
	return &value
}

func (f fieldImpl) String() string {
	return reflect.Indirect(f.value).String()
}

func (f fieldImpl) PointerBool() *bool {
	if f.value.IsNil() {
		return nil
	}
	value := f.Bool()
	return &value
}

func (f fieldImpl) Bool() bool {
	return reflect.Indirect(f.value).Bool()
}

func (f fieldImpl) PointerTime() *time.Time {
	if f.value.IsNil() {
		return nil
	}
	value := f.Time()
	return &value
}

func (f fieldImpl) Time() time.Time {
	value, ok := reflect.Indirect(f.value).Interface().(time.Time)
	if !ok {
		panic(fmt.Sprintf(`field "%s" is not instance of time.Time`, f.field.Name))
	}

	return value
}

func (f fieldImpl) Interface() interface{} {
	return f.value.Interface()
}
