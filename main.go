package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/sap/hana_sql_exporter/cmd"
)

func main() {
	Formatter := new(log.TextFormatter)
	Formatter.TimestampFormat = "02-01-2006 15:04:05"
	log.SetFormatter(&log.TextFormatter{
		DisableColors:   true,
		FullTimestamp:   true,
		TimestampFormat: "02-01-2006 15:04:05",
	})

	cmd.Root()
}
