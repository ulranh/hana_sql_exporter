package cmd_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/ulranh/hana_sql_exporter/cmd"

	"github.com/stretchr/testify/assert"
)

// func (config *cmd.Config) getTestData1(mPos, tPos int) []metricRecord {
// 	mr := []metricRecord{
// 		metricRecord{
// 			999.0,
// 			[]string{"l" + strconv.Itoa(mPos) + strconv.Itoa(tPos)},
// 			[]string{"lv" + strconv.Itoa(mPos) + strconv.Itoa(tPos)},
// 		},
// 	}
// 	return mr
// }

// 0 metrics, 0 tenants
func Test_CollectMetrics00(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(0, 1)
	config.DataFunc = config.GetTestData1

	res := config.CollectMetrics()
	assert.Nil(res)
}

// 0 metrics, 1 tenants
func Test_CollectMetrics01(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(0, 1)
	config.DataFunc = config.GetTestData1

	res := config.CollectMetrics()
	assert.Nil(res)
}

// 1 metrics, 0 tenants
func Test_CollectMetrics10(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(1, 0)
	config.DataFunc = config.GetTestData1

	res := config.CollectMetrics()
	assert.Nil(res)
}

// 1 tenant, 1 metric
func Test_CollectMetrics11(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(1, 1)
	config.DataFunc = config.GetTestData1

	res := config.CollectMetrics()
	// assert.Nil(res)
	assert.Equal(res, []cmd.MetricData{cmd.MetricData{Name: "m1", Help: "h1", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l00"}, LabelValues: []string{"lv00"}}}}})
}

// 2 metrics, 1 tenant
func Test_CollectMetrics21(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(2, 1)
	config.DataFunc = config.GetTestData1

	res := config.CollectMetrics()
	fmt.Println("21: ", res)
	fmt.Println("21: ", []cmd.MetricData{cmd.MetricData{Name: "m1", Help: "h1", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l00"}, LabelValues: []string{"lv00"}}}}, cmd.MetricData{Name: "m2", Help: "h2", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l10"}, LabelValues: []string{"lv10"}}}}})
	assert.Equal(true, cmp.Equal(res, []cmd.MetricData{cmd.MetricData{Name: "m1", Help: "h1", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l00"}, LabelValues: []string{"lv00"}}}}, cmd.MetricData{Name: "m2", Help: "h2", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l10"}, LabelValues: []string{"lv10"}}}}}))
}

// 1 metric, 2 tenants
func Test_CollectMetrics12(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(1, 2)
	config.DataFunc = config.GetTestData1

	res := config.CollectMetrics()
	fmt.Println("12: ", res)
	fmt.Println("12: ", []cmd.MetricData{cmd.MetricData{Name: "m1", Help: "h1", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l01"}, LabelValues: []string{"lv01"}}, cmd.MetricRecord{Value: 999, Labels: []string{"l00"}, LabelValues: []string{"lv00"}}}}})
	assert.Equal(true, cmp.Equal(res, []cmd.MetricData{cmd.MetricData{Name: "m1", Help: "h1", MetricType: "gauge", Stats: []cmd.MetricRecord{cmd.MetricRecord{Value: 999, Labels: []string{"l01"}, LabelValues: []string{"lv01"}}, cmd.MetricRecord{Value: 999, Labels: []string{"l00"}, LabelValues: []string{"lv00"}}}}}))
}

func Test_CollectNilMetrics(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(2, 3)
	config.DataFunc = config.GetTestData2

	res := config.CollectMetrics()
	assert.Nil(res)
}

func Test_GetMetricRows(t *testing.T) {

	assert := assert.New(t)

	config := getTestConfig(1, 1)
	ti := config.Tenants[0]

	// rows.Columns
	rows := &sql.Rows{}
	_, err := ti.GetMetricRows(rows)
	assert.NotNil(err)
}

func Test_GetSelection(t *testing.T) {
	assert := assert.New(t)

	// tag not in tagfilter
	config := getTestConfig(3, 3)
	res := config.GetSelection(2, 2)
	assert.Equal(res, "")

	// only selects are allowed
	config = getTestConfig(4, 1)
	res = config.GetSelection(3, 0)
	assert.Equal(res, "")

	// metrics schema filter must include a tenant schema
	config = getTestConfig(2, 2)
	res = config.GetSelection(1, 1)
	assert.Equal(res, "")

	config = getTestConfig(1, 1)
	res = config.GetSelection(0, 0)
	assert.Equal(res, "select count(*) from sys.m_blocked_transactions")
}

func Test_AdaptSchemaFilter(t *testing.T) {

	var mi = []cmd.MetricInfo{
		cmd.MetricInfo{
			SchemaFilter: []string{},
		},
		cmd.MetricInfo{
			SchemaFilter: []string{},
		},
	}
	var config = cmd.Config{
		Metrics: mi,
	}

	assert := assert.New(t)

	config.AdaptSchemaFilter()
	for _, metric := range mi {
		b := cmd.ContainsString("sys", metric.SchemaFilter)
		assert.Equal(b, true)
	}
}

func Test_ContainsString(t *testing.T) {
	assert := assert.New(t)

	slice := []string{"S1", "s2", "S3", "s4"}
	resSlice := []bool{true, false, false}

	for i, s := range []string{"s3", "s5", ""} {

		b := cmd.ContainsString(s, slice)
		assert.Equal(b, resSlice[i])
	}

	b := cmd.ContainsString("S1", []string{})
	assert.Equal(b, false)
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

		b := cmd.SubSliceInSlice(slice, refSlice)
		assert.Equal(b, resSlice[i])
	}

	b := cmd.SubSliceInSlice([]string{"s1", "s2"}, []string{})
	assert.Equal(b, false)
}

func Test_FirstValueInSclice(t *testing.T) {
	assert := assert.New(t)

	refSlice := []string{"S1", "S2", "S2"}
	slices := [][]string{
		[]string{"s1", "s2", "s2"},
		[]string{"s2", "s2"},
		[]string{},
	}
	resSlice := []string{"s1", "s2", ""}

	for i, slice := range slices {

		s := cmd.FirstValueInSlice(slice, refSlice)
		assert.Equal(s, resSlice[i])
	}

	b := cmd.FirstValueInSlice([]string{"s1", "s2"}, []string{})
	assert.Equal(b, "")
}
