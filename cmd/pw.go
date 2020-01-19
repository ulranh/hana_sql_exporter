package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
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

	newSecret, err := newSecret(secret, flags["tenant"], pw)
	if err != nil {
		return errors.Wrap(err, " pw - newSecret")
	}

	// marshal the secret map
	config.Secret, err = proto.Marshal(&newSecret)
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
func newSecret(secret internal.Secret, tenants *string, pw []byte) (internal.Secret, error) {

	encPw, err := internal.PwEncrypt(pw, secret.Name["secretkey"])
	if err != nil {
		return internal.Secret{}, errors.Wrap(err, "newSecret - PwEncrypt ")
	}

	// insert tenants given by flag -tenant and created pw in secret map
	for _, tName := range strings.Split(*tenants, ",") {
		secret.Name[strings.ToLower(tName)] = encPw
	}

	return secret, nil
}
