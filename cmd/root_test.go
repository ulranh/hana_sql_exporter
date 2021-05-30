package cmd_test

import "github.com/ulranh/hana_sql_exporter/cmd"

func getTestConfig(mCnt, tCnt int) *cmd.Config {
	mi := []cmd.MetricInfo{
		{
			Name:         "m1",
			Help:         "h1",
			MetricType:   "gauge",
			SQL:          "select count(*) from <SCHEMA>.m_blocked_transactions",
			SchemaFilter: []string{"sys"},
		},
		{
			Name:         "m2",
			Help:         "h2",
			MetricType:   "gauge",
			SQL:          "select allocated_size,port from <SCHEMA>.m_rs_memory where category='TABLE'",
			SchemaFilter: []string{"sys"},
		},
		{
			Name:       "m3",
			Help:       "h3",
			MetricType: "gauge",
			SQL:        "select top 1 (case when active_status = 'YES' then 1 else -1 end), database_name from <SCHEMA>.m_databases",
			TagFilter:  []string{"erp"},
		},
		{
			Name:       "m4",
			Help:       "h4",
			MetricType: "gauge",
			SQL:        "update",
		},
	}
	ti := []cmd.TenantInfo{
		{
			Name:    "d01",
			Schemas: []string{"sys"},
		},
		{
			Name: "D02",
		},
		{
			Name: "d03",
			Tags: []string{"bw"},
		},
	}
	config := cmd.Config{
		Metrics: mi[:mCnt],
		Tenants: ti[:tCnt],
		Timeout: 3,
	}
	return &config
}
