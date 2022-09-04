package dynamicstruct

import (
	"testing"
	"time"
)

type (
	benchmarkStruct struct {
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

	benchmarkPartialStructOne struct {
		String   string
		Integer  int
		Uinteger uint
		Float    float64
		Bool     bool
		Time     time.Time
	}

	benchmarkPartialStructTwo struct {
		PointerString   *string
		PointerInteger  *int
		PointerUinteger *uint
		PointerFloat    *float64
		PointerBool     *bool
		PointerTime     *time.Time
		Integers        []int
	}
)

func BenchmarkClassicWay_NewInstance(b *testing.B) {
	for i := 0; i < b.N; i++ {
		newInstance()
	}
}

func BenchmarkNewStruct_NewInstance(b *testing.B) {
	integer := 0
	uinteger := uint(0)
	str := ""
	float := 0.0
	boolean := false

	dStruct := NewStruct().
		AddField("Integer", integer, "").
		AddField("String", str, "").
		AddField("Uinteger", uinteger, "").
		AddField("Float", float, "").
		AddField("Bool", boolean, "").
		AddField("Time", time.Time{}, "").
		AddField("PointerString", &str, "").
		AddField("PointerInteger", &integer, "").
		AddField("PointerUinteger", &uinteger, "").
		AddField("PointerFloat", &float, "").
		AddField("PointerBool", &boolean, "").
		AddField("PointerTime", &time.Time{}, "").
		AddField("Integers", []int{}, "").
		Build()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dStruct.New()
	}
}

func BenchmarkNewStruct_NewInstance_Parallel(b *testing.B) {
	integer := 0
	uinteger := uint(0)
	str := ""
	float := 0.0
	boolean := false

	dStruct := NewStruct().
		AddField("Integer", integer, "").
		AddField("String", str, "").
		AddField("Uinteger", uinteger, "").
		AddField("Float", float, "").
		AddField("Bool", boolean, "").
		AddField("Time", time.Time{}, "").
		AddField("PointerString", &str, "").
		AddField("PointerInteger", &integer, "").
		AddField("PointerUinteger", &uinteger, "").
		AddField("PointerFloat", &float, "").
		AddField("PointerBool", &boolean, "").
		AddField("PointerTime", &time.Time{}, "").
		AddField("Integers", []int{}, "").
		Build()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dStruct.New()
		}
	})
}

func BenchmarkExtendStruct_NewInstance(b *testing.B) {
	integer := 0
	uinteger := uint(0)
	str := ""
	float := 0.0
	boolean := false

	dStruct := ExtendStruct(benchmarkPartialStructOne{}).
		AddField("PointerString", &str, "").
		AddField("PointerInteger", &integer, "").
		AddField("PointerUinteger", &uinteger, "").
		AddField("PointerFloat", &float, "").
		AddField("PointerBool", &boolean, "").
		AddField("PointerTime", &time.Time{}, "").
		AddField("Integers", []int{}, "").
		Build()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dStruct.New()
	}
}

func BenchmarkExtendStruct_NewInstance_Parallel(b *testing.B) {
	integer := 0
	uinteger := uint(0)
	str := ""
	float := 0.0
	boolean := false

	dStruct := ExtendStruct(benchmarkPartialStructOne{}).
		AddField("PointerString", &str, "").
		AddField("PointerInteger", &integer, "").
		AddField("PointerUinteger", &uinteger, "").
		AddField("PointerFloat", &float, "").
		AddField("PointerBool", &boolean, "").
		AddField("PointerTime", &time.Time{}, "").
		AddField("Integers", []int{}, "").
		Build()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dStruct.New()
		}
	})
}

func BenchmarkMergeStructs_NewInstance(b *testing.B) {
	dStruct := MergeStructs(benchmarkPartialStructOne{}, benchmarkPartialStructTwo{}).Build()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dStruct.New()
	}
}

func BenchmarkMergeStructs_NewInstance_Parallel(b *testing.B) {
	dStruct := MergeStructs(benchmarkPartialStructOne{}, benchmarkPartialStructTwo{}).Build()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dStruct.New()
		}
	})
}

func newInstance() benchmarkStruct {
	return benchmarkStruct{}
}
