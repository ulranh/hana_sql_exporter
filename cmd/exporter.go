package cmd

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SAP/go-hdb/driver"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/hana_sql_exporter/internal"
)

type collector struct {
	// possible metric descriptions.
	Desc *prometheus.Desc

	// a parameterized function used to gather metrics.
	stats func() []metric
}

type metric struct {
	name   string
	help   string
	metric *metricInfo
	stats  []*metricData
}

type metricData struct {
	value       float64
	labels      []string
	labelValues []string
}

// start collector and web server
func (config *Config) web(flags map[string]*string) error {

	// unmarshal secret byte array
	var secret internal.Secret
	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		return errors.Wrap(err, " unable to unmarshal secret")
	}

	// expand tenant information
	for i := len(config.Tenants) - 1; i >= 0; i-- {

		pw, err := getPW(secret, config.Tenants[i].Name)
		if err != nil {
			log.WithFields(log.Fields{
				"tenant": config.Tenants[i].Name,
				"error":  err,
			}).Error("Can't find or decrypt password for tenant - tenant removed")

			// remove tenant from slice
			config.Tenants = append(config.Tenants[:i], config.Tenants[i+1:]...)
			continue
		}

		// connect to db tenant
		connector := driver.NewBasicAuthConnector(config.Tenants[i].ConnStr, config.Tenants[i].User, pw)
		connector.SetTimeout(60)
		config.Tenants[i].conn = sql.OpenDB(connector)
		defer config.Tenants[i].conn.Close()

		// get tenant usage and hana-user schema information
		err = config.Tenants[i].collectRemainingTenantInfos()
		if err != nil {
			log.WithFields(log.Fields{
				"tenant": config.Tenants[i].Name,
				"error":  err,
			}).Error("Problems with select of remaining tenant info - tenant removed.")

			// remove tenant from slice
			config.Tenants = append(config.Tenants[:i], config.Tenants[i+1:]...)
			continue
		}
	}

	// add sys schema to SchemaFilter if it does not exists
	for _, m := range config.Metrics {
		if !containsString("sys", m.SchemaFilter) {
			m.SchemaFilter = append(m.SchemaFilter, "sys")
		}
	}

	stats := func() []metric {
		data := config.collectMetrics()
		return data
	}

	// start collector
	c := newCollector(stats)
	prometheus.MustRegister(c)

	// start http server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:         ":" + *flags["port"],
		Handler:      mux,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}
	err := server.ListenAndServe()
	if err != nil {
		return errors.Wrap(err, " web - ListenAndServe")
	}
	return nil
}

// start collecting all metrics and fetch the results
func (config *Config) collectMetrics() []metric {

	// start := time.Now()
	// log.WithFields(log.Fields{
	// 	"timestamp": start,
	// }).Info("Start scraping")

	resC := make(chan metric)
	go func(config *Config) {
		var wg sync.WaitGroup

		for _, m := range config.Metrics {
			wg.Add(1)

			go func(config *Config, m *metricInfo) {
				defer wg.Done()
				resC <- metric{
					name:   m.Name,
					help:   m.Help,
					metric: m,
					stats:  config.collectTenantsMetric(m),
				}
			}(config, m)
		}
		wg.Wait()
		close(resC)
	}(config)

	var ms []metric
	for m := range resC {
		ms = append(ms, m)
	}

	// log.WithFields(log.Fields{
	// 	"timestamp": time.Since(start),
	// }).Info("Finish scraping")
	// log.Info("------------------------------------------------------------------------------")
	return ms
}

// start collecting metric information for all tenants
func (config *Config) collectTenantsMetric(m *metricInfo) []*metricData {
	// start := time.Now()
	// log.WithFields(log.Fields{
	// 	"metric":    m.Name,
	// 	"timestamp": start,
	// }).Info("Start")

	resC := make(chan []*metricData)

	go func(config *Config, m *metricInfo) {
		var wg sync.WaitGroup

		for _, t := range config.Tenants {
			wg.Add(1)

			go func(m *metricInfo, t *tenantInfo) {
				defer wg.Done()

				resC <- prepareMetricTenantData(m, t)
			}(m, t)

		}
		wg.Wait()
		close(resC)
	}(config, m)

	var metricData []*metricData
	for v := range resC {
		if v != nil {
			metricData = append(metricData, v...)
		}
	}
	// log.WithFields(log.Fields{
	// 	"metric":    m.Name,
	// 	"timestamp": time.Since(start),
	// }).Info("Finish")

	return metricData
}

// filter out not associated tenants
func prepareMetricTenantData(m *metricInfo, t *tenantInfo) []*metricData {

	// all values of metrics tag filter must be in tenants tags, otherwise the
	// metric is not relevant for the tenant
	if !subSliceInSlice(m.TagFilter, t.Tags) {
		return nil
	}

	sel := strings.TrimSpace(m.SQL)
	if !strings.EqualFold(sel[0:6], "select") {
		log.WithFields(log.Fields{
			"metric": m.Name,
			"tenant": t.Name,
		}).Error("Only selects are allowed")

	}

	// metrics schema filter must include a tenant schema
	var schema string
	if schema = firstValueInSlice(m.SchemaFilter, t.schemas); 0 == len(schema) {
		log.WithFields(log.Fields{
			"metric": m.Name,
			"tenant": t.Name,
		}).Error("SchemaFilter value in toml file is missing")
		return nil
	}
	sel = strings.ReplaceAll(sel, "<SCHEMA>", schema)

	res, err := t.getMetricTenantData(sel)

	if err != nil {
		log.WithFields(log.Fields{
			"metric": m.Name,
			"tenant": t.Name,
			"error":  err,
		}).Error("Can't get sql result for metric")
		return nil
	}
	return res
}

// get metric data for one tenant
func (t *tenantInfo) getMetricTenantData(sel string) ([]*metricData, error) {
	var err error

	var rows *sql.Rows
	rows, err = t.conn.Query(sel)
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

	// first column must not be string -> do better
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

	var md []*metricData
	for rows.Next() {
		data := metricData{
			labels:      []string{"tenant", "usage"},
			labelValues: []string{strings.ToLower(t.Name), strings.ToLower(t.usage)},
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
		md = append(md, &data)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "GetSqlData - rows")
	}
	return md, nil
}

func newCollector(stats func() []metric) *collector {
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
				valueType[strings.ToLower(mi.metric.MetricType)],
				v.value,
				v.labelValues...,
			)
			ch <- m
		}
	}
}

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

func containsString(str string, slice []string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, str) {
			return true
		}
	}
	return false
}

// true if every item in sublice exists in slice or sublice is empty
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
