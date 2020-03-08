package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/hana_sql_exporter/internal"
	"golang.org/x/crypto/ssh/terminal"
)

// save password of tenant(s) database user
func (config *Config) pw(flags map[string]*string) error {

	fmt.Print("Password: ")
	pw, err := terminal.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, " pw - ReadPassword")
	}

	// fill map with existing secrets from configfile
	var secret internal.Secret
	if err = proto.Unmarshal(config.Secret, &secret); err != nil {
		return errors.Wrap(err, " tenant  - Unmarshal")
	}

	// create secret key once if it doesn't exist
	if _, ok := secret.Name["secretkey"]; !ok {

		secret.Name = make(map[string][]byte)
		secret.Name["secretkey"], err = internal.GetSecretKey()
		if err != nil {
			return errors.Wrap(err, "passwd - getPassword")
		}
	}

	err = config.newSecret(secret, flags["tenant"], pw)
	if err != nil {
		return errors.Wrap(err, " pw - newSecret")
	}

	// marshal the secret map
	config.Secret, err = proto.Marshal(&secret)
	if err != nil {
		return errors.Wrap(err, " ImportTenants - Marshal")
	}

	// write changes to configfile
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(config); err != nil {
		return errors.Wrap(err, " ImportTenants - Marshal")
	}

	f, err := os.Create(*flags["config"])
	if err != nil {
		return errors.Wrap(err, "tomlExport - os.Create")
	}
	_, err = f.WriteString(buf.String())
	if err != nil {
		return errors.Wrap(err, "tomlExport - f.WriteString")
	}

	return nil
}

// create encrypted secret map for tenant(s)
func (config *Config) newSecret(secret internal.Secret, tenants *string, pw []byte) error {

	encPw, err := internal.PwEncrypt(pw, secret.Name["secretkey"])
	if err != nil {
		return errors.Wrap(err, "newSecret - PwEncrypt ")
	}

	tMap := make(map[string]bool)
	for _, tenant := range strings.Split(*tenants, ",") {
		tMap[strings.ToLower(tenant)] = false
	}
	for _, tenant := range config.Tenants {
		tName := strings.ToLower(tenant.Name)

		// check if pw system exists in configfile system slice
		if _, ok := tMap[tName]; !ok {
			continue
		}
		tMap[tName] = true

		// connection test
		db := dbConnect(tenant.ConnStr, tenant.User, string(pw))
		defer db.Close()

		if err := db.Ping(); err != nil {
			log.WithFields(log.Fields{
				"tenant": tenant.Name,
			}).Error("Can't connect to tenant with given parameters.")
			continue
		}

		// add password to secret map
		secret.Name[tName] = encPw
	}

	for k, v := range tMap {
		if !v {
			log.WithFields(log.Fields{
				"tenant": k,
			}).Error("Did not find tenant in configfile tenants slice.")
		}
	}

	return nil
}
