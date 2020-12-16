// Copyright © 2020 Ulrich Anhalt <ulrich.anhalt@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type collector struct {
	// possible metric descriptions.
	Desc *prometheus.Desc

	// a parameterized function used to gather metrics.
	stats func() []MetricData
}

type MetricData struct {
	Name       string
	Help       string
	MetricType string
	Stats      []MetricRecord
}

type MetricRecord struct {
	Value       float64
	Labels      []string
	LabelValues []string
}

// webCmd represents the web command
var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the exporter",
	Long: `With the command web you can start the hana sql exporter. For example:
	hana_sql_exporter web
	hana_sql_exporter web --config ./hana_sql_exporter.toml`,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := getConfig()
		if err != nil {
			exit("Can't handle config file: ", err)
		}

		config.Timeout, err = cmd.Flags().GetUint("timeout")
		if err != nil {
			exit("Problem with timeout flag: ", err)
		}
		config.port, err = cmd.Flags().GetString("port")
		if err != nil {
			exit("Problem with port flag: ", err)
		}

		// set data func
		config.DataFunc = config.GetMetricData

		err = config.Web()
		if err != nil {
			exit("Can't call exporter: ", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(webCmd)

	webCmd.PersistentFlags().UintP("timeout", "t", 5, "scrape timeout of the hana_sql_exporter in seconds.")
	webCmd.PersistentFlags().StringP("port", "p", "9658", "port, the hana_sql_exporter listens to.")
}

// create new collector
func newCollector(stats func() []MetricData) *collector {
	return &collector{
		stats: stats,
	}
}

// Describe - describe implements prometheus.Collector.
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// Collect - implements prometheus.Collector.
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	// take a stats snapshot. must be concurrency safe.
	stats := c.stats()

	var valueType = map[string]prometheus.ValueType{
		"gauge":   prometheus.GaugeValue,
		"counter": prometheus.CounterValue,
	}

	for _, mi := range stats {
		for _, v := range mi.Stats {
			m := prometheus.MustNewConstMetric(
				prometheus.NewDesc(mi.Name, mi.Help, v.Labels, nil),
				valueType[strings.ToLower(mi.MetricType)],
				v.Value,
				v.LabelValues...,
			)
			ch <- m
		}
	}
}

// Web - start collector and web server
func (config *Config) Web() error {
	var err error

	config.Tenants, err = config.prepare()
	if err != nil {
		exit("Preparation of tenants not possible: ", err)
	}

	// close tenant connections at the end
	for i := range config.Tenants {
		defer config.Tenants[i].conn.Close()
	}

	stats := func() []MetricData {
		return config.CollectMetrics()
	}

	// start collector
	c := newCollector(stats)
	prometheus.MustRegister(c)

	// start http server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", RootHandler)

	// Add the pprof routes
	// mux.HandleFunc("/debug/pprof/", pprof.Index)
	// mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	// mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	// mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	// mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	// mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	// mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	// mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	server := &http.Server{
		Addr:         ":" + config.port,
		Handler:      mux,
		WriteTimeout: time.Duration(config.Timeout+2) * time.Second,
		ReadTimeout:  time.Duration(config.Timeout+2) * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		return errors.Wrap(err, "web(ListenAndServe)")
	}
	return nil
}

func RootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "prometheus hana_sql_exporter: please call <host>:<port>/metrics")
}

// CollectMetrics - collecting all metrics and fetch the results
func (config *Config) CollectMetrics() []MetricData {

	var wg sync.WaitGroup
	metricCnt := len(config.Metrics)
	metricsC := make(chan MetricData, metricCnt)

	for mPos := range config.Metrics {

		wg.Add(1)
		go func(mPos int) {

			defer wg.Done()
			metricsC <- MetricData{
				Name:       config.Metrics[mPos].Name,
				Help:       config.Metrics[mPos].Help,
				MetricType: config.Metrics[mPos].MetricType,
				Stats:      config.CollectMetric(mPos),
			}
		}(mPos)
	}

	go func() {
		wg.Wait()
		close(metricsC)
	}()

	var metricsData []MetricData
	for metric := range metricsC {
		if metric.Stats != nil {
			metricsData = append(metricsData, metric)
		}
	}

	return metricsData
}

// CollectMetric - collecting one metric for every tenants
func (config *Config) CollectMetric(mPos int) []MetricRecord {

	// set timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(config.Timeout)*time.Second))
	defer cancel()

	tenantCnt := len(config.Tenants)
	metricC := make(chan []MetricRecord, tenantCnt)

	for tPos := range config.Tenants {

		go func(tPos int) {

			metricC <- config.DataFunc(mPos, tPos)
		}(tPos)
	}

	// collect data
	var sData []MetricRecord
	for i := 0; i < tenantCnt; i++ {
		select {
		case mc := <-metricC:
			if mc != nil {
				sData = append(sData, mc...)
			}
		case <-ctx.Done():
			return sData
		}
	}
	return sData
}

// GetMetricData - metric data for one tenant
func (config *Config) GetMetricData(mPos, tPos int) []MetricRecord {

	sel := config.GetSelection(mPos, tPos)
	if "" == sel {
		return nil
	}

	rows, err := config.Tenants[tPos].conn.Query(sel)
	if err != nil {
		log.WithFields(log.Fields{
			"metric": config.Metrics[mPos].Name,
			"tenant": config.Tenants[tPos].Name,
			"error":  err,
		}).Error("Can't get sql result for metric")
		return nil
	}
	defer rows.Close()

	md, err := config.Tenants[tPos].GetMetricRows(rows)
	if err = rows.Err(); err != nil {
		return nil
	}
	return md
}

// GetSelection - prepare the db selection
func (config *Config) GetSelection(mPos, tPos int) string {

	// all values of metrics tag filter must be in tenants tags, otherwise the
	// metric is not relevant for the tenant
	if !SubSliceInSlice(config.Metrics[mPos].TagFilter, config.Tenants[tPos].Tags) {
		return ""
	}

	sel := strings.TrimSpace(config.Metrics[mPos].SQL)
	if !strings.EqualFold(sel[0:6], "select") {
		log.WithFields(log.Fields{
			"metric": config.Metrics[mPos].Name,
			"tenant": config.Tenants[tPos].Name,
		}).Error("Only selects are allowed")
		return ""
	}

	// metrics schema filter must include a tenant schema
	var schema string
	if schema = FirstValueInSlice(config.Metrics[mPos].SchemaFilter, config.Tenants[tPos].Schemas); 0 == len(schema) {
		log.WithFields(log.Fields{
			"metric": config.Metrics[mPos].Name,
			"tenant": config.Tenants[tPos].Name,
		}).Error("metrics schema filter must include a tenant schema")
		return ""
	}
	return strings.ReplaceAll(config.Metrics[mPos].SQL, "<SCHEMA>", schema)
}

// GetMetricRows - return the metric values
func (tenant *TenantInfo) GetMetricRows(rows *sql.Rows) ([]MetricRecord, error) {

	cols, err := rows.Columns()
	if err != nil {
		return nil, errors.Wrap(err, "GetMetricRows(rows.Columns)")
	}

	if len(cols) < 1 {
		return nil, errors.New("GetMetricRows(no columns)")
	}

	// first column must not be string
	colt, err := rows.ColumnTypes()
	if err != nil {
		return nil, errors.Wrap(err, "GetMetricRows(rows.ColumnTypes)")
	}
	switch colt[0].ScanType().Name() {
	case "string", "bool", "":
		return nil, errors.New("GetMetricRows(first column must be numeric)")
	default:
	}

	values := make([]sql.RawBytes, len(cols))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	var md []MetricRecord
	for rows.Next() {
		data := MetricRecord{
			Labels:      []string{"tenant", "usage"},
			LabelValues: []string{strings.ToLower(tenant.Name), strings.ToLower(tenant.Usage)},
		}
		err = rows.Scan(scanArgs...)
		if err != nil {
			return nil, errors.Wrap(err, "GetMetricRows(rows.Scan)")
		}

		for i, colval := range values {

			// check for NULL value
			if colval == nil {
				return nil, errors.Wrap(err, "GetMetricRows(colval is null)")
			}

			if 0 == i {

				// the first column must be the float value
				data.Value, err = strconv.ParseFloat(string(colval), 64)
				if err != nil {
					return nil, errors.Wrap(err, "GetMetricRows(ParseFloat - first column cannot be converted to float64)")
				}
			} else {
				data.Labels = append(data.Labels, strings.ToLower(cols[i]))
				data.LabelValues = append(data.LabelValues, strings.ToLower(strings.Join(strings.Split(string(colval), " "), "_")))

			}
		}
		md = append(md, data)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "GetMetricRows(rows)")
	}

	return md, nil
}

// add missing information to tenant struct
func (config *Config) prepare() ([]TenantInfo, error) {

	var tenantsOk []TenantInfo

	// adapt config.Metrics schema filter
	config.AdaptSchemaFilter()

	secretMap, err := config.GetSecretMap()
	if err != nil {
		return nil, errors.Wrap(err, "prepare(getSecretMap)")
	}

	for i := 0; i < len(config.Tenants); i++ {

		config.Tenants[i].conn = config.getConnection(i, secretMap)
		if config.Tenants[i].conn == nil {
			continue
		}

		// get tenant usage and hana-user schema information
		err = config.collectRemainingTenantInfos(i)
		if err != nil {
			log.WithFields(log.Fields{
				"tenant": config.Tenants[i].Name,
				"error":  err,
			}).Error("Problems with select of remaining tenant info - tenant removed!")

			continue
		}
		tenantsOk = append(tenantsOk, config.Tenants[i])
	}
	return tenantsOk, nil

}

// get tenant usage and hana-user schema information
func (config *Config) collectRemainingTenantInfos(tPos int) error {

	// get tenant usage information
	row := config.Tenants[tPos].conn.QueryRow("select usage from sys.m_database")
	err := row.Scan(&config.Tenants[tPos].Usage)
	if err != nil {
		return errors.Wrap(err, "collectRemainingTenantInfos(Scan)")
	}

	// append sys schema to tenant schemas
	config.Tenants[tPos].Schemas = append(config.Tenants[tPos].Schemas, "sys")

	// append remaining user schema privileges
	rows, err := config.Tenants[tPos].conn.Query("select schema_name from sys.granted_privileges where object_type='SCHEMA' and grantee=$1", strings.ToUpper(config.Tenants[tPos].User))
	if err != nil {
		return errors.Wrap(err, "collectRemainingTenantInfos(Query)")
	}
	defer rows.Close()

	for rows.Next() {
		var schema string
		err := rows.Scan(&schema)
		if err != nil {
			return errors.Wrap(err, "collectRemainingTenantInfos(Scan)")
		}
		config.Tenants[tPos].Schemas = append(config.Tenants[tPos].Schemas, schema)
	}
	if err = rows.Err(); err != nil {
		return errors.Wrap(err, "collectRemainingTenantInfos(rows.Err)")
	}
	return nil
}

//  AdaptSchemaFilter - add sys schema to SchemaFilter if it does not exists
func (config *Config) AdaptSchemaFilter() {

	for mPos := range config.Metrics {
		if !ContainsString("sys", config.Metrics[mPos].SchemaFilter) {
			config.Metrics[mPos].SchemaFilter = append(config.Metrics[mPos].SchemaFilter, "sys")
		}
	}
}

// ContainsString - true, if slice contains string
func ContainsString(str string, slice []string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, str) {
			return true
		}
	}
	return false
}

// SubSliceInSlice - true, if every item in sublice exists in slice or sublice is empty
func SubSliceInSlice(subSlice []string, slice []string) bool {
	for _, vs := range subSlice {
		for _, v := range slice {
			if strings.EqualFold(vs, v) {
				goto nextCheck
			}
		}
		return false
	nextCheck:
	}
	return true
}

// FirstValueInSlice - return first sublice value that exists in slice
func FirstValueInSlice(subSlice []string, slice []string) string {
	for _, vs := range subSlice {
		for _, v := range slice {
			if strings.EqualFold(vs, v) {
				return vs
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------
// GetTestData1 - for testing purpose only
func (config *Config) GetTestData1(mPos, tPos int) []MetricRecord {
	mr := []MetricRecord{
		MetricRecord{
			999.0,
			[]string{"l" + strconv.Itoa(mPos) + strconv.Itoa(tPos)},
			[]string{"lv" + strconv.Itoa(mPos) + strconv.Itoa(tPos)},
		},
	}
	return mr
}

// GetTestData2 - for testing purpose only
func (config *Config) GetTestData2(mPos, tPos int) []MetricRecord {
	return nil
}
