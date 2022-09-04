package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInSliceString(t *testing.T) {
	sl := []string{"a", "b", "c"}
	require.Equal(t, true, InSliceString(sl, "b"))

	sl = []string{"a", "a", "a"}
	require.Equal(t, true, InSliceString(sl, "a"))

	sl = []string{"a", "b", "c"}
	require.Equal(t, false, InSliceString(sl, "d"))
}

func TestGetOutBoundIP(t *testing.T) {
	fmt.Println(GetOutBoundIP())
	//require.Equal()
}

func TestGetPrivateIP(t *testing.T) {
	fmt.Println(GetPrivateIP())
	//require.Equal()
}

func TestGetPublicIP(t *testing.T) {
	fmt.Println(GetPublicIP())
}

func BenchmarkGetPrivateIP(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		GetPrivateIP()
	}
}

func BenchmarkGetPublicIP(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		GetPublicIP()
	}
}

func BenchmarkGetOutBoundIP(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		GetOutBoundIP()
	}
}