package cmd

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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
	stats      []statData
}

type statData struct {
	value       float64
	labels      []string
	labelValues []string
}

// start collector and web server
func (config *Config) web(flags map[string]*string) error {

	var err error

	// tenant connections must be closed at the end
	for _, t := range config.Tenants {
		defer t.conn.Close()
	}

	stats := func() []metricData {
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
	err = server.ListenAndServe()
	if err != nil {
		return errors.Wrap(err, " web - ListenAndServe")
	}
	return nil
}

// start collecting all metrics and fetch the results
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

// start collecting metric information for all tenants
func (config *Config) collectMetric(mPos int) []statData {

	metricC := make(chan []statData, len(config.Tenants))

	for tPos := range config.Tenants {

		go func(tPos int) {
			metricC <- config.prepareMetricData(mPos, tPos)
		}(tPos)
	}

	i := 0
	var sData []statData
	timeAfter := time.After(time.Duration(config.timeout) * time.Second)

stopReading:
	for {
		select {
		case mc := <-metricC:
			if mc != nil {
				sData = append(sData, mc...)
			}
			i += 1
			if len(config.Tenants) == i {
				break stopReading
			}
		case <-timeAfter:
			break stopReading
		}
	}
	return sData
}

// filter out not associated tenants
func (config *Config) prepareMetricData(mPos, tPos int) []statData {

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
// !!!!!!!!!!!!!!!!! ist tenant pointer oke ????
func (tenant *tenantInfo) getMetricData(sel string) ([]statData, error) {
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

	var md []statData
	for rows.Next() {
		data := statData{
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
