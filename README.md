## SAP Hana SQL Exporter for Prometheus  [![CircleCI](https://circleci.com/gh/ulranh/hana_sql_exporter/tree/master.svg?style=svg)](https://circleci.com/gh/ulranh/hana_sql_exporter) [![Go Report Card](https://goreportcard.com/badge/github.com/ulranh/hana_sql_exporter)](https://goreportcard.com/report/github.com/ulranh/hana_sql_exporter)  [![Docker Pulls](https://img.shields.io/docker/pulls/ulranh/hana-sql-exporter)](https://hub.docker.com/r/ulranh/hana-sql-exporter)

The purpose of this exporter is to support monitoring SAP and SAP HanaDB  instances with [Prometheus](https://prometheus.io) and [Grafana](https://grafana.com).

## Installation
The exporter can be downloaded as [released Version](https://github.com/ulranh/hana_sql_exporter/releases/latest) or build with:

```
$ git clone git@github.com:ulranh/hana_sql_exporter.git
$ cd hana_sql_exporter
$ go build
```
## Preparation

#### Database User
A database user is necessary for every tenant with read access for all affected schemas:

```
# Login with authorized user:
$ create user <user> password <pw> no force_first_password_change;
$ alter user <user> disable password lifetime;
$ grant catalog read to <user>;

# Login with authorized user:
$ grant select on schema <schema> to <user>;
# <schema>: _SYS_STATISTICS, SAPABAP1, SAPHANADB ... 
```

#### Configfile
The next necessary piece is a [toml](https://github.com/toml-lang/toml) configuration file where the encrypted passwords, tenants and metrics are stored. The expected default name is hana_sql_exporter.toml and the expected default location of this file is the home directory of the user. The flag -config can be used to assign other locations or names.

The file contains a Tenants slice followed by a Metrics Slice:

```
[[Tenants]]
  Name = "q01"
  Tags = ["abap", "ewm"]
  ConnString = "hanaq01.example.com:32041"
  User = "dbuser1"

[[Tenants]]
  Name = "q02"
  Tags = ["abap", "erp"]
  ConnString = "hanaqj1.example.com:31044"
  User = "dbuser2"

[[Metrics]]
  Name = "hdb_backup_status"
  Help = "Status of last hana backup."
  MetricType = "gauge"
  TagFilter = []
  SchemaFilter = [] # the sys schema will be added automatically
  SQL = "select (case when state_name = 'successful' then 0 when state_name = 'running' then 1 else -1 end) as val, entry_type_name as type from <SCHEMA>.m_backup_catalog where entry_id in (select max(entry_id) from m_backup_catalog group by entry_type_name)"

[[Metrics]]
  Name = "hdb_cancelled_jobs"
  Help = "Sap jobs with status cancelled/aborted (today)"
  MetricType = "counter"
  TagFilter = ["abap"]
  SchemaFilter = ["sapabap1", "sapabap","sapewm"]
  SQL = "select count(*) from <SCHEMA>.tbtco where enddate=current_utcdate and status='A'"
```

Below is a description of the tenant and metric struct fields:

#### Tenant information

| Field      | Type         | Description | Example |
| ---------- | ------------ |------------ | ------- |
| Name       | string       | SAP Hana tenant name | "P01", "q02" |
| Tags       | string array | Tags describing the system | ["abap", "erp"], ["systemdb"], ["java"] |
| ConnString | string       | Connection string \<hostname\>:\<tenant sql port\> - the sql port can be selected in the following way on the system db: "select database_name,sql_port from sys_databases.m_services"  | "host.domain:31041" | 
| User       | string       | Tenant database user name | |

#### Metric information

| Field        | Type         | Description | Example |
| ------------ | ------------ |------------ | ------- |
| Name         | string       | Metric name | "hdb_info" |
| Help         | string       | Metric help text | "Hana database version and uptime|
| MetricType   | string       | Type of metric | "counter" or "gauge" |
| TagFilter    | string array | The metric will only be executed, if all values correspond with the existing tenant tags | TagFilter ["abap", "erp"] needs at least tenant Tags ["abap", "erp"] otherwise the metric will not be used |
| SchemaFilter | string array | The metric will only be used, if the tenant user has one of schemas in SchemaFilter assigned. The first matching schema will be replaced with the <SCHEMA> placeholder of the select.  | ["sapabap1", "sapewm"] |
| SQL          | string       | The select is responsible for the data retrieval. Conventionally the first column must represent the value of the metric. The folowing columns are used as labels and must be string values. | "select days_between(start_time, current_timestamp) as uptime, version from \<SCHEMA\>.m_database" (SCHEMA uppercase) |

#### Database passwords

With the following commands the passwords for the example tenants above can be written to the Secret section of the configfile:
```
$ ./hana_sql_exporter pw -tenant q01 -config ./hana_sql_exporter.toml
$ ./hana_sql_exporter pw -tenant qj1 -config ./hana_sql_exporter.toml
```
With one password for multiple tenants, the following notation is also possible:
```
$ ./hana_sql_exporter pw -tenant q01,qj1 -config ./hana_sql_exporter.toml
```

## Usage

Now the web server can be started:
#### Binary

The default port is 9658 which can be changed with the -port flag.
```
$ ./hana_sql_exporter web -config ./hana_sql_exporter.toml
```

#### Docker
The Docker image can be downloaded from Docker Hub or build with the Dockerfile. Then it can be started as follows:
```
$ docker run -d --name=hana_exporter --restart=always -p 9658:9658 -v /home/<user>/hana_sql_exporter.toml:/app/hana_sql_exporter.toml \<image name\>
```
#### Kubernetes
An example config can be found in the examples folder. First of all create a sap namespace. Then apply the created configfile above as configmap and start the deployment:
```
$ kubectl apply -f sap-namespace.yaml 
$ kubectl create configmap hana-config -n sap --from-file ./hana_sql_exporter.toml -o yaml
$ kubectl apply -f hana-deployment.yaml
```

Configfile changes can be applied in the following way:
```
$ kubectl create configmap hana-config -n sap --from-file ./hana_sql_exporter.toml -o yaml --dry-run | sudo kubectl replace -f -
$ kubectl scale --replicas=0 -n sap deployment hana-sql-exporter
$ kubectl scale --replicas=1 -n sap deployment hana-sql-exporter
```
#### Prometheus configfile
The necessary entries in the prometheus configfile can look something like the following:
```
  - job_name: hana-exporter
        scrape_interval: 60s
        static_configs:
          - targets: ['172.45.111.105:9658']
            labels:  {'instance': 'hana-exporter-test'}
          - targets: ['hana-exporter.sap.svc.cluster.local:9658']
            labels:  {'instance': 'hana-exporter-dev'}
```

## Result
The resulting information can be found in the Prometheus expression browser and can be used as normal for creating alerts or displaying dashboards in Grafana.

The image below shows for example the duration of all complete data backups. With one dashboard it is possible to detect hanging or aborted backups of all systems:

 ![backups](/examples/images/backups.png)
