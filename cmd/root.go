package cmd

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
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
type tenantsInfo []*tenantInfo

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
	Tenants tenantsInfo
	Metrics []*metricInfo
	timeout uint64
}

// help text for command usage
const doc = `usage: hana_sql_exporter <command> <param> [param ...]

These are the commands:
  pw              Set password for tenant connection(s)
  web             Start Prometheus web client

Run 'hana_sql_exporter <command> -help' to see the corresponding parameters.
`

var (
	errCmdNotGiven       = errors.New("\nCmd Problem: command ist not given.")
	errCmdNotAvailable   = errors.New("\nCmd Problem: command ist not available.")
	errCmdFlagMissing    = errors.New("\nCmd Problem: a required cmd flag is missing.")
	errCmdFileMissing    = errors.New("\nCmd Problem: no configfile found.")
	errConfSecretMissing = errors.New("\nthe secret info is missing. Please add the passwords with \"hana_sql_exporter pw -tenant <tenant>\"")
	errConfTenantMissing = errors.New("\nthe tenant info is missing.")
	errConfMetricMissing = errors.New("\nthe metric info is missing.")
	errConfTenant        = errors.New("\nat least one of the required tenant fields Name,ConnStr or User is empty.")
	errConfMetric        = errors.New("\nat least one of the required metric fields Name,Help,MetricType or SQL is empty.")
	errConfMetricType    = errors.New("\nat least one of the existing MetricType fields does not contain the allowed content counter or gauge.")
)

// map of allowed parameters
var paramMap = map[string]*struct {
	value string
	usage string
}{
	"tenant": {
		value: "",
		usage: "Name(s) of tenant(s) - lower case words separated by comma (required)\ne.g. -tenant P01,P02",
	},
	"port": {
		value: "9658",
		usage: "Client port",
	},
	"config": {
		value: "",
		usage: "Path + name of toml config file",
	},
	"timeout": {
		value: "2",
		usage: "timeout of the hana connector in seconds",
	},
}

// map of allowed commands
var commandMap = map[string]struct {
	defaultParams map[string]string
	params        []string
}{
	"pw": {
		params: []string{"config", "tenant"},
	},
	"web": {
		params: []string{"config", "port", "timeout"},
	},
}

// Root checks given command, flags and config file. If ok jump to corresponding execution
func Root() {

	// check command and flags
	command, flags, err := getCmdInfo(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// decode config file
	var config Config
	if _, err := toml.DecodeFile(*flags["config"], &config); err != nil {
		exit(fmt.Sprint("Problem with configfile decoding: ", err))
	}

	// parse config file
	if err = config.parseConfigInfo(command); err != nil {
		exit(fmt.Sprint("Problem with configfile content: ", err))
	}
	// run cmd
	var cmdFunc = map[string]func(map[string]*string) error{
		"pw":  config.pw,
		"web": config.web,
	}

	err = cmdFunc[command](flags)
	if err != nil {
		exit(fmt.Sprint("Error from command with ", err))
	}
}

// check command and args
func getCmdInfo(args []string) (string, map[string]*string, error) {

	// no command given
	if 1 == len(args) {
		fmt.Print(doc)
		return "", nil, errCmdNotGiven
	}

	// command name is incorrect
	if _, ok := commandMap[args[1]]; !ok {
		fmt.Print(doc)
		return "", nil, errCmdNotAvailable
	}

	// initial path for config file
	home, err := homedir.Dir()
	if err != nil {
		return "", nil, errors.Wrap(err, "can't detect homdir for config file")
	}
	paramMap["config"].value = path.Join(home, args[0]+".toml")

	// create and fill flag set
	flags := make(map[string]*string)
	c := flag.NewFlagSet(args[1], flag.ExitOnError)
	for _, v := range commandMap[args[1]].params {
		flags[v] = c.String(v, paramMap[v].value, paramMap[v].usage)
	}
	c.SetOutput(os.Stderr)

	// parse flags
	err = c.Parse(args[2:])
	if err != nil {
		return "", nil, errors.Wrap(err, "getCmdInfo - problem with c.Parse()")
	}
	if c.Parsed() {

		// break if default flag tenant for command pw ist missing
		if name, ok := flags["tenant"]; ok {
			if *name == "" {
				c.PrintDefaults()
				return "", nil, errCmdFlagMissing
			}
		}

		// break if there exists no configfile at the given default path
		if name, ok := flags["config"]; ok {
			if _, err := os.Stat(*name); os.IsNotExist(err) {
				c.PrintDefaults()
				return "", nil, errCmdFileMissing
			}
		}
	}

	return args[1], flags, nil
}

// check configfile
func (config *Config) parseConfigInfo(cmd string) error {
	if cmd != "web" {
		return nil
	}

	if 0 == len(config.Secret) {
		return errConfSecretMissing
	}
	if 0 == len(config.Tenants) {
		return errConfTenantMissing
	}
	if 0 == len(config.Metrics) {
		return errConfMetricMissing
	}
	for _, tenant := range config.Tenants {
		if 0 == len(tenant.Name) || 0 == len(tenant.ConnStr) || 0 == len(tenant.User) {
			return errConfTenant
		}
	}
	for _, metric := range config.Metrics {
		if 0 == len(metric.Name) || 0 == len(metric.Help) || 0 == len(metric.MetricType) || 0 == len(metric.SQL) {
			return errConfMetric
		}
		if !strings.EqualFold(metric.MetricType, "counter") && !strings.EqualFold(metric.MetricType, "gauge") {
			return errConfMetricType
		}
	}
	return nil
}

func exit(msg string) {
	fmt.Println(msg)
	os.Exit(1)
}
