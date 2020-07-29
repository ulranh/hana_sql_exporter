package cmd

import (
	"database/sql"
	"strings"

	goHdbDriver "github.com/SAP/go-hdb/driver"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/hana_sql_exporter/internal"
)

// add missing information to tenant struct
func (config *Config) prepareTenants() ([]tenantInfo, error) {

	var tenantsOk []tenantInfo

	// unmarshal secret byte array
	var secret internal.Secret
	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		return nil, errors.Wrap(err, " unable to unmarshal secret")
	}

	for i := 0; i < len(config.Tenants); i++ {

		pw, err := getPW(secret, config.Tenants[i].Name)
		if err != nil {
			log.WithFields(log.Fields{
				"tenant": config.Tenants[i].Name,
				"error":  err,
			}).Error("Can't find or decrypt password for tenant - tenant removed!")

			continue
		}

		// connect to db tenant
		config.Tenants[i].conn = dbConnect(config.Tenants[i].ConnStr, config.Tenants[i].User, pw)
		err = dbPing(config.Tenants[i].Name, config.Tenants[i].conn)
		if err != nil {
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

// add sys schema to SchemaFilter if it does not exists
func (config *Config) adaptSchemaFilter() {

	for mPos := range config.Metrics {
		if !containsString("sys", config.Metrics[mPos].SchemaFilter) {
			config.Metrics[mPos].SchemaFilter = append(config.Metrics[mPos].SchemaFilter, "sys")
		}
	}
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

// connect to database
func dbConnect(connStr, user, pw string) *sql.DB {

	connector, err := goHdbDriver.NewDSNConnector("hdb://" + user + ":" + pw + "@" + connStr)
	if err != nil {
		log.Fatal(err)
	}
	// connector.SetTimeout(timeout)
	return sql.OpenDB(connector)
}

// check if ping works
func dbPing(name string, conn *sql.DB) error {
	err := conn.Ping()
	if err != nil {
		log.WithFields(log.Fields{
			"tenant": name,
			"error":  err,
		}).Error("Can't connect to tenant")
		return err
	}
	return nil
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
