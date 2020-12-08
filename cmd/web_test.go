package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_firstValueInSclice(t *testing.T) {
	assert := assert.New(t)

	refSlice := []string{"S1", "S2", "S2"}
	slices := [][]string{
		[]string{"s1", "s2", "s2"},
		[]string{"s2", "s2"},
		[]string{},
	}
	resSlice := []string{"s1", "s2", ""}

	for i, slice := range slices {

		s := firstValueInSlice(slice, refSlice)
		assert.Equal(s, resSlice[i])
	}

	b := firstValueInSlice([]string{"s1", "s2"}, []string{})
	assert.Equal(b, "")
}

func Test_subSliceInSclice(t *testing.T) {
	assert := assert.New(t)

	refSlice := []string{"S1", "s2", "S3", "s4"}
	slices := [][]string{
		[]string{"s1", "S2", "s3", "S4"},
		[]string{"S2", "S4"},
		[]string{"s8", "S2", "S5"},
		[]string{"s0", "S5", "s7"},
		[]string{},
	}
	resSlice := []bool{true, true, false, false, true}

	for i, slice := range slices {

		b := subSliceInSlice(slice, refSlice)
		assert.Equal(b, resSlice[i])
	}

	b := subSliceInSlice([]string{"s1", "s2"}, []string{})
	assert.Equal(b, false)
}

func Test_containsString(t *testing.T) {
	assert := assert.New(t)

	slice := []string{"S1", "s2", "S3", "s4"}
	resSlice := []bool{true, false, false}

	for i, s := range []string{"s3", "s5", ""} {

		b := containsString(s, slice)
		assert.Equal(b, resSlice[i])
	}

	b := containsString("S1", []string{})
	assert.Equal(b, false)
}

func Test_adaptSchemaFilter(t *testing.T) {

	var mi = []metricInfo{
		metricInfo{
			SchemaFilter: []string{},
		},
		metricInfo{
			SchemaFilter: []string{},
		},
	}
	var config = Config{
		Metrics: mi,
	}

	assert := assert.New(t)

	config.adaptSchemaFilter()
	for _, metric := range mi {
		b := containsString("sys", metric.SchemaFilter)
		assert.Equal(b, true)
	}
}
