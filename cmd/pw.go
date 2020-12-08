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
	crypt "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ulranh/hana_sql_exporter/internal"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/ssh/terminal"
)

// pwCmd represents the pw command
var pwCmd = &cobra.Command{
	Use:   "pw",
	Short: "Set passwords for the tenants in the config file",
	Long: `With the command pw you can set the passwords for the tenants you want to monitor. You can set the password for one tenant or several tenants separated by comma. For example:
	hana_sql_exporter pw --tenant d01
	hana_sql_exporter pw -t d01,d02 --config ./.hana_sql_exporter.toml`,
	Run: func(cmd *cobra.Command, args []string) {

		config, err := getConfig()
		if err != nil {
			exit("Can't handle config file: ", err)
		}

		err = config.setPw(cmd)
		if err != nil {
			exit("Can't set password: ", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(pwCmd)

	pwCmd.PersistentFlags().StringP("tenant", "t", "", "name(s) of tenant(s) separated by comma")
	pwCmd.MarkPersistentFlagRequired("tenant")
}

// save password(s) of tenant(s) database user to the config file
func (config *Config) setPw(cmd *cobra.Command) error {

	fmt.Print("Password: ")
	pw, err := terminal.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, "setPw(ReadPassword)")
	}

	tenants, err := cmd.Flags().GetString("tenant")
	if err != nil {
		return errors.Wrap(err, "setPw(GetString)")
	}

	// secret, err := config.addSecret(tenants, pw)
	config.Secret, err = config.addSecret(tenants, pw)
	if err != nil {
		return errors.Wrap(err, "setPw(newSecret)")
	}

	viper.Set("secret", config.Secret)
	err = viper.WriteConfig()
	if err != nil {
		return errors.Wrap(err, "setPw(WriteConfig)")
	}

	// connection test for all tenants
	secretMap, err := config.getSecretMap()
	if err != nil {
		return errors.Wrap(err, "prepare(getSecretMap)")
	}
	for _, tenant := range config.Tenants {
		pw, err := getPw(secretMap, tenant.Name)
		if err != nil {
			return errors.Wrap(err, "setPw(getPw)")
		}
		db := dbConnect(tenant.ConnStr, tenant.User, pw)
		defer db.Close()

		if err := dbPing(tenant.Name, db); err != nil {
			log.WithFields(log.Fields{
				"tenant": tenant.Name,
			}).Error("Cannot ping tenant. Perhaps wrong password?")
		}
	}

	return nil
}

// create encrypted secret map for tenant(s)
func (config *Config) addSecret(tenants string, pw []byte) ([]byte, error) {
	var err error

	// fill map with existing secrets from configfile
	var secret internal.Secret
	if err = proto.Unmarshal(config.Secret, &secret); err != nil {
		return nil, errors.Wrap(err, "newSecret(Unmarshal)")
	}

	// create secret key once if it doesn't exist
	if _, ok := secret.Name["secretkey"]; !ok {

		secret.Name = make(map[string][]byte)
		secret.Name["secretkey"], err = getSecretKey()
		if err != nil {
			return nil, errors.Wrap(err, "newSecret(getSecretKey)")
		}
	}

	// encrypt password
	encPw, err := pwEncrypt(pw, secret.Name["secretkey"])
	if err != nil {
		return nil, errors.Wrap(err, "newSecret(PwEncrypt)")
	}

	for _, tenant := range strings.Split(tenants, ",") {
		tenant := strings.ToLower(tenant)

		// check, if cmd line tenant exists in configfile
		tInfo := config.findTenant(tenant)
		if "" == tInfo.Name {
			log.WithFields(log.Fields{
				"tenant": tenant,
			}).Error("missing tenant")
			return nil, errors.New("Did not find tenant in configfile tenants slice.")
		}

		// add password to secret map
		secret.Name[tenant] = encPw
	}

	// write pw information back to the config file
	newSecret, err := proto.Marshal(&secret)
	if err != nil {
		return nil, errors.Wrap(err, "newSecret(Marshal)")
	}

	return newSecret, nil
}

// findTenant - check if cmpTenant already exists in configfile
func (config *Config) findTenant(cmpTenant string) tenantInfo {
	for _, tenant := range config.Tenants {
		if strings.ToLower(tenant.Name) == strings.ToLower(cmpTenant) {
			return tenant
		}
	}
	return tenantInfo{}
}

// GetSecretKey - create secret key once
func getSecretKey() ([]byte, error) {

	key := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	if _, err := rand.Read(key); err != nil {
		return nil, errors.Wrap(err, "GetSecretKey(rand.Read)")
	}

	return key, nil
}

// PwEncrypt - encrypt tenant password
func pwEncrypt(bytePw, byteSecret []byte) ([]byte, error) {

	var secretKey [32]byte
	copy(secretKey[:], byteSecret)

	var nonce [24]byte
	if _, err := io.ReadFull(crypt.Reader, nonce[:]); err != nil {
		return nil, errors.Wrap(err, "PwEncrypt(ReadFull)")
	}

	return secretbox.Seal(nonce[:], bytePw, &nonce, &secretKey), nil
}

// PwDecrypt - decrypt tenant password
func pwDecrypt(encrypted, byteSecret []byte) (string, error) {

	var secretKey [32]byte
	copy(secretKey[:], byteSecret)

	var decryptNonce [24]byte
	copy(decryptNonce[:], encrypted[:24])
	decrypted, ok := secretbox.Open(nil, encrypted[24:], &decryptNonce, &secretKey)
	if !ok {
		return "", errors.New("PwDecrypt(secretbox.Open)")
	}

	return string(decrypted), nil
}

// check if connection works
func (config *Config) getSecretMap() (internal.Secret, error) {

	// unmarshal secret byte array
	var secret internal.Secret
	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		return internal.Secret{}, errors.Wrap(err, "prepare(Unmarshal)")
	}
	return secret, nil
}

// getPw - decrypt password
func getPw(secret internal.Secret, tenant string) (string, error) {

	tenant = strings.ToLower(tenant)

	// get encrypted tenant pw
	if _, ok := secret.Name[tenant]; !ok {
		return "", errors.New("encrypted tenant pw info does not exist")
	}

	// decrypt tenant password
	pw, err := pwDecrypt(secret.Name[tenant], secret.Name["secretkey"])
	if err != nil {
		return "", errors.Wrap(err, "getPW(PwDecrypt)")
	}
	return pw, nil
}
