package cmd

import (
	"testing"

	"github.com/ulranh/sap/hana_sql_exporter/internal"
)

func Test_firstValueInSclice(t *testing.T) {
	slice := []string{"S1", "S2", "S2"}
	slice1 := []string{"s1", "s2", "s2"}
	slice2 := []string{"s2", "s2"}
	slice3 := []string{}

	s := firstValueInSlice(slice1, slice)
	internal.Equals(t, s, "s1")
	s = firstValueInSlice(slice2, slice)
	internal.Equals(t, s, "s2")
	s = firstValueInSlice(slice3, slice)
	internal.Equals(t, s, "")
}

func Test_subSliceInSlice(t *testing.T) {
	slice := []string{"S1", "s2", "S3", "s4"}
	slice1 := []string{"s1", "S2", "s3", "S4"}
	slice2 := []string{"S2", "S4"}
	slice3 := []string{"s8", "S2", "S5"}
	slice4 := []string{"s0", "S5", "s7"}
	slice5 := []string{}

	b := subSliceInSlice(slice1, slice)
	internal.Equals(t, b, true)
	b = subSliceInSlice(slice2, slice)
	internal.Equals(t, b, true)
	b = subSliceInSlice(slice3, slice)
	internal.Equals(t, b, false)
	b = subSliceInSlice(slice4, slice)
	internal.Equals(t, b, false)
	b = subSliceInSlice(slice5, slice)
	internal.Equals(t, b, true)
}

func Test_containsString(t *testing.T) {
	slice := []string{"S1", "s2", "S3", "s4"}

	b := containsString("s3", slice)
	internal.Equals(t, b, true)
	b = containsString("s5", slice)
	internal.Equals(t, b, false)
	b = containsString("", slice)
	internal.Equals(t, b, false)
}
