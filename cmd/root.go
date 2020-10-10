// Copyright © 2020 NAME HERE <EMAIL ADDRESS>
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
	"os"
	"time"

	goHdbDriver "github.com/SAP/go-hdb/driver"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// tenant info
type tenantInfo struct {
	Name    string
	Tags    []string
	ConnStr string
	User    string
	usage   string
	schemas []string
	conn    *sql.DB
}

// metric info
type metricInfo struct {
	Name         string
	Help         string
	MetricType   string
	TagFilter    []string
	SchemaFilter []string
	SQL          string
}

// Config struct with config file infos
type Config struct {
	Secret  []byte
	Tenants []tenantInfo
	Metrics []metricInfo
	timeout uint
	port    string
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

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.hana_sql_exporter.toml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
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
		return nil, errors.Wrap(err, "getConfig(Unarshal)")
	}
	return &config, nil
}

// exit program with error message
func exit(msg string, err error) {
	fmt.Println(msg, err)
	os.Exit(1)
}

// add sys schema to SchemaFilter if it does not exists
func (config *Config) adaptSchemaFilter() {

	for mPos := range config.Metrics {
		if !containsString("sys", config.Metrics[mPos].SchemaFilter) {
			config.Metrics[mPos].SchemaFilter = append(config.Metrics[mPos].SchemaFilter, "sys")
		}
	}
}

// connect to hana database
func dbConnect(connStr, user, pw string) *sql.DB {

	connector, err := goHdbDriver.NewDSNConnector("hdb://" + user + ":" + pw + "@" + connStr)
	if err != nil {
		log.Fatal(err)
	}
	connector.SetTimeout(10)

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db
}

// check if connection works
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
