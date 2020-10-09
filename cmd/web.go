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

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/ulranh/hana_sql_exporter/internal"
)

type collector struct {
	// possible metric descriptions.
	Desc *prometheus.Desc

	// a parameterized function used to gather metrics.
	stats func() []metricData
}

type metricData struct {
	name       string
	help       string
	metricType string
	stats      []metricRecord
}

type metricRecord struct {
	value       float64
	labels      []string
	labelValues []string
}

// webCmd represents the web command
var webCmd = &cobra.Command{
	Use:   "web",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		// fmt.Println("web called")
		config, err := getConfig()
		if err != nil {
			exit("web - getConfig", err)
		}

		// fmt.Println(cmd.Flags().GetString("port"))
		// fmt.Println(cmd.Flags().GetUint("timeout"))
		config.timeout, err = cmd.Flags().GetUint("timeout")
		if err != nil {
			exit("web - GetUint(timeout)", err)
		}
		config.port, err = cmd.Flags().GetString("port")
		if err != nil {
			exit("web - GetUint(port)", err)
		}

		err = config.web()
		if err != nil {
			exit(" pw - setPw", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(webCmd)

	webCmd.PersistentFlags().Uint("timeout", 5, "scrape timeout of the hana_sql_exporter in seconds.")
	// webCmd.MarkPersistentFlagRequired("timeout")
	webCmd.PersistentFlags().String("port", "9658", "port, the hana_sql_exporter listens to.")
	// webCmd.MarkPersistentFlagRequired("port")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// webCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// webCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func newCollector(stats func() []metricData) *collector {
	return &collector{
		stats: stats,
	}
}

// describe implements prometheus.Collector.
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// collect implements prometheus.Collector.
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	// take a stats snapshot. must be concurrency safe.
	stats := c.stats()

	var valueType = map[string]prometheus.ValueType{
		"gauge":   prometheus.GaugeValue,
		"counter": prometheus.CounterValue,
	}

	for _, mi := range stats {
		for _, v := range mi.stats {
			m := prometheus.MustNewConstMetric(
				prometheus.NewDesc(mi.name, mi.help, v.labels, nil),
				valueType[strings.ToLower(mi.metricType)],
				v.value,
				v.labelValues...,
			)
			ch <- m
		}
	}
}

// start collector and web server
func (config *Config) web() error {
	var err error

	config.Tenants, err = config.prepare()
	if err != nil {
		exit(" preparation of tenants not possible", err)
	}

	// close tenant connections at the end
	for _, t := range config.Tenants {
		defer t.conn.Close()
	}

	stats := func() []metricData {
		return config.collectMetrics()
		// data := config.collectMetrics()
		// return data
	}

	// start collector
	c := newCollector(stats)
	prometheus.MustRegister(c)

	// start http server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", rootHandler)

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
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		return errors.Wrap(err, " web - ListenAndServe")
	}
	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "prometheus hana_sql_exporter: please call <host>:<port>/metrics")
}

// collecting all metrics and fetch the results
func (config *Config) collectMetrics() []metricData {

	var wg sync.WaitGroup

	metricsC := make(chan metricData, len(config.Metrics))
	for mPos := range config.Metrics {

		wg.Add(1)
		go func(mPos int) {

			defer wg.Done()
			metricsC <- metricData{
				name:       config.Metrics[mPos].Name,
				help:       config.Metrics[mPos].Help,
				metricType: config.Metrics[mPos].MetricType,
				stats:      config.collectMetric(mPos),
			}
		}(mPos)
	}

	go func() {
		wg.Wait()
		close(metricsC)
	}()

	var metricsData []metricData
	for metric := range metricsC {
		metricsData = append(metricsData, metric)
	}

	return metricsData
}

func (config *Config) collectMetric(mPos int) []metricRecord {

	tenantCnt := len(config.Tenants)
	metricC := make(chan []metricRecord, tenantCnt)

	// set timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(config.timeout)*time.Second))
	defer cancel()

	for tPos := range config.Tenants {

		go func(tPos int) {
			metricC <- config.prepareMetricData(mPos, tPos)
		}(tPos)
	}

	// collect data
	var sData []metricRecord
	for i := 0; i < tenantCnt; i++ {
		select {
		case mc := <-metricC:
			if mc != nil {
				sData = append(sData, mc...)
			}
			// fmt.Println("OK", i)
		case <-ctx.Done():
			// fmt.Println("TIMEOUT!")
			return sData
		}
	}
	return sData
}

// filter out not associated tenants
func (config *Config) prepareMetricData(mPos, tPos int) []metricRecord {

	// !!!!!!!!!!!!!!!
	// t := rand.Intn(5)
	// time.Sleep(time.Duration(t) * time.Second)

	// all values of metrics tag filter must be in tenants tags, otherwise the
	// metric is not relevant for the tenant
	if !subSliceInSlice(config.Metrics[mPos].TagFilter, config.Tenants[tPos].Tags) {
		return nil
	}

	sel := strings.TrimSpace(config.Metrics[mPos].SQL)
	if !strings.EqualFold(sel[0:6], "select") {
		log.WithFields(log.Fields{
			"metric": config.Metrics[mPos].Name,
			"tenant": config.Tenants[tPos].Name,
		}).Error("Only selects are allowed")
		return nil
	}

	// metrics schema filter must include a tenant schema
	var schema string
	if schema = firstValueInSlice(config.Metrics[mPos].SchemaFilter, config.Tenants[tPos].schemas); 0 == len(schema) {
		log.WithFields(log.Fields{
			"metric": config.Metrics[mPos].Name,
			"tenant": config.Tenants[tPos].Name,
		}).Error("SchemaFilter value in toml file is missing")
		return nil
	}
	sel = strings.ReplaceAll(sel, "<SCHEMA>", schema)

	// if metric is relevant, check connection
	// err := dbPing(config.Tenants[tPos].Name, config.Tenants[tPos].conn)
	// if err != nil {
	// 	return nil
	// }

	// res, err := config.getMetricData(tPos, sel)
	res, err := config.Tenants[tPos].getMetricData(sel)
	if err != nil {
		log.WithFields(log.Fields{
			"metric": config.Metrics[mPos].Name,
			"tenant": config.Tenants[tPos].Name,
			"error":  err,
		}).Error("Can't get sql result for metric")
		return nil
	}
	return res
}

// get metric data for one tenant
func (tenant *tenantInfo) getMetricData(sel string) ([]metricRecord, error) {
	var err error

	var rows *sql.Rows

	rows, err = tenant.conn.Query(sel)
	if err != nil {
		return nil, errors.Wrap(err, "GetSqlData - query")
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, errors.Wrap(err, "GetSqlData - columns")
	}

	if len(cols) < 1 {
		return nil, errors.Wrap(err, "GetSqlData - no columns")
	}

	// first column must not be string
	colt, err := rows.ColumnTypes()
	if err != nil {
		return nil, errors.Wrap(err, "GetSqlData - columnTypes")
	}
	switch colt[0].ScanType().Name() {
	case "string", "bool", "":
		return nil, errors.New("GetSqlData - first column must be numeric")
	default:
	}

	values := make([]sql.RawBytes, len(cols))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	var md []metricRecord
	for rows.Next() {
		data := metricRecord{
			labels:      []string{"tenant", "usage"},
			labelValues: []string{strings.ToLower(tenant.Name), strings.ToLower(tenant.usage)},
		}
		err = rows.Scan(scanArgs...)
		if err != nil {
			return nil, errors.Wrap(err, "GetSqlData - rows.Scan")
		}

		for i, colval := range values {

			// check for NULL value
			if colval == nil {
				return nil, errors.Wrap(err, "GetSqlData - colval is null")
			}

			if 0 == i {

				// the first column must be the float value
				data.value, err = strconv.ParseFloat(string(colval), 64)
				if err != nil {
					return nil, errors.Wrap(err, "GetSqlData - first column cannot be converted to float64")
				}
			} else {
				data.labels = append(data.labels, strings.ToLower(cols[i]))
				data.labelValues = append(data.labelValues, strings.ToLower(strings.Join(strings.Split(string(colval), " "), "_")))

			}
		}
		md = append(md, data)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "GetSqlData - rows")
	}
	return md, nil
}

// add missing information to tenant struct
func (config *Config) prepare() ([]tenantInfo, error) {

	// var err error
	var tenantsOk []tenantInfo

	// add timeout value to config struct
	// config.timeout, err = strconv.ParseUint(*flags["timeout"], 10, 0)
	// if err != nil {
	// 	fmt.Println(" timeout flag has wrong type", err)
	// 	os.Exit(1)
	// }

	// adapt config.Metrics schema filter
	config.adaptSchemaFilter()

	// unmarshal secret byte array
	var secret internal.Secret
	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		return nil, errors.Wrap(err, " unable to unmarshal secret")
	}

	for i := 0; i < len(config.Tenants); i++ {

		pw, err := getPW(secret, strings.ToLower(config.Tenants[i].Name))
		if err != nil {
			log.WithFields(log.Fields{
				"tenant": config.Tenants[i].Name,
				"error":  err,
			}).Error("Can't find or decrypt password for tenant - tenant removed!")

			continue
		}

		// connect to db tenant
		config.Tenants[i].conn = dbConnect(config.Tenants[i].ConnStr, config.Tenants[i].User, pw)
		if err = dbPing(config.Tenants[i].Name, config.Tenants[i].conn); err != nil {
			continue
		}

		// get tenant usage and hana-user schema information
		err = config.Tenants[i].collectRemainingTenantInfos()
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
func (t *tenantInfo) collectRemainingTenantInfos() error {

	// get tenant usage information
	row := t.conn.QueryRow("select usage from sys.m_database")
	err := row.Scan(&t.usage)
	if err != nil {
		return err
	}

	// append sys schema to tenant schemas
	t.schemas = append(t.schemas, "sys")

	// append remaining user schema privileges
	rows, err := t.conn.Query("select schema_name from sys.granted_privileges where object_type='SCHEMA' and grantee=$1", strings.ToUpper(t.User))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var schema string
		err := rows.Scan(&schema)
		if err != nil {
			return err
		}
		t.schemas = append(t.schemas, schema)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return nil
}

// decrypt password
func getPW(secret internal.Secret, name string) (string, error) {

	// get encrypted tenant pw
	if _, ok := secret.Name[name]; !ok {
		return "", errors.New("encrypted tenant pw info does not exist")
	}

	// decrypt tenant password
	pw, err := internal.PwDecrypt(secret.Name[name], secret.Name["secretkey"])
	if err != nil {
		return "", err
	}
	return pw, nil
}

// true, if slice contains string
func containsString(str string, slice []string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, str) {
			return true
		}
	}
	return false
}

// true, if every item in sublice exists in slice or sublice is empty
func subSliceInSlice(subSlice []string, slice []string) bool {
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

// return first sublice value that exists in slice
func firstValueInSlice(subSlice []string, slice []string) string {
	for _, vs := range subSlice {
		for _, v := range slice {
			if strings.EqualFold(vs, v) {
				return vs
			}
		}
	}
	return ""
}
