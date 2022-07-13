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
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	goHdbDriver "github.com/SAP/go-hdb/driver"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ulranh/hana_sql_exporter/internal"
)

// TenantInfo - tennant data
type TenantInfo struct {
	Name    string
	Tags    []string
	ConnStr string
	User    string
	Usage   string
	Schemas []string
	conn    *sql.DB
}

// MetricInfo - metric data
type MetricInfo struct {
	Name         string
	Help         string
	MetricType   string
	TagFilter    []string
	SchemaFilter []string
	SQL          string
}

// Config struct with config file infos
type Config struct {
	Secret   []byte
	Tenants  []TenantInfo
	Metrics  []MetricInfo
	DataFunc func(mPos, tPos int) []MetricRecord
	Timeout  uint
	port     string
}

var cfgFile string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "hana_sql_exporter",
	Short: "The purpose of this hana_sql_exporter is to support monitoring SAP and SAP HanaDB instances with Prometheus and Grafana.",
	Long:  `The purpose of the hana_sql_exporter is to support monitoring SAP and SAP HanaDB instances with Prometheus and Grafana. As the name suggests, with the hana_sql_exporter a sql select is responsible for the data retrieval. By definition the first column must represent the value of the metric. The following columns are used as labels and must be string values.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		exit("RootCmd can't be executed: ", err)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.hana_sql_exporter.toml)")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			exit("Homedir can't be found: ", err)
		}

		// Search config in home directory with name ".hana_sql_exporter" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("toml")
		viper.SetConfigName(".hana_sql_exporter.toml")
	}

	// viper.AutomaticEnv() // read in environment variables that match

}

// read and unmarshal configfile into Config struct
func getConfig() (*Config, error) {
	var config Config

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		return nil, errors.Wrap(err, "getConfig(ReadInConfig)")
	}

	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "getConfig(Unmarshal)")
	}

	return &config, nil
}

// exit program with error message
func exit(msg string, err error) {
	fmt.Println(msg, err)
	os.Exit(1)
}

// prepare, establish, check and return connection to hana db
func (config *Config) getConnection(tId int, secretMap internal.Secret) *sql.DB {

	pw, err := GetPassword(secretMap, config.Tenants[tId].Name)
	if err != nil {
		log.WithFields(log.Fields{
			"tenant": config.Tenants[tId].Name,
		}).Error("Cannot find password for tenant.")
		return nil
	}
	db := config.dbConnect(tId, pw)
	if db == nil {
		log.WithFields(log.Fields{
			"tenant": config.Tenants[tId].Name,
		}).Error("Can't get connection.")
		return nil
	}
	// defer db.Close()

	if err := db.Ping(); err != nil {
		log.WithFields(log.Fields{
			"tenant": config.Tenants[tId].Name,
		}).Error("Cannot ping tenant. Perhaps wrong password?")
		return nil
	}
	return db
}

// connect to hana db
func (config *Config) dbConnect(tId int, pw string) *sql.DB {

	dsn := fmt.Sprintf("hdb://%s:%s@%s",
		config.Tenants[tId].User,
		url.QueryEscape(pw),
		config.Tenants[tId].ConnStr)

	connector, err := goHdbDriver.NewDSNConnector(dsn)

	if err != nil {
		return nil
	}
	connector.SetTimeout(time.Duration(config.Timeout) * time.Second)

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db
}

func low(str string) string {
	return strings.TrimSpace(strings.ToLower(str))
}
