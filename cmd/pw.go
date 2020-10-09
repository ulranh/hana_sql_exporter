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
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ulranh/hana_sql_exporter/internal"
	"golang.org/x/crypto/ssh/terminal"
)

// pwCmd represents the pw command
var pwCmd = &cobra.Command{
	Use:   "pw",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {

		config, err := getConfig()
		if err != nil {
			exit(" pw - getConfig", err)
		}

		err = config.setPw(cmd)
		if err != nil {
			exit(" pw - setPw", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(pwCmd)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// pwCmd.PersistentFlags().String("foo", "", "A help for foo")

	pwCmd.PersistentFlags().String("tenant", "", "name(s) of tenant(s) - lower case words separated by comma e.g. --tenant P01,P02")
	pwCmd.MarkPersistentFlagRequired("tenant")
	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// pwCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// save password(s) of tenant(s) database user
func (config *Config) setPw(cmd *cobra.Command) error {

	fmt.Print("Password: ")
	pw, err := terminal.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, " pw - ReadPassword")
	}

	tenants, err := cmd.Flags().GetString("tenant")
	if err != nil {
		return errors.Wrap(err, " setPw - GetString(tenant)")
	}

	err = config.newSecret(tenants, pw)
	if err != nil {
		return errors.Wrap(err, " pw - newSecret")
	}

	return nil
}

// create encrypted secret map for tenant(s)
func (config *Config) newSecret(tenants string, pw []byte) error {
	var err error

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

	encPw, err := internal.PwEncrypt(pw, secret.Name["secretkey"])
	if err != nil {
		return errors.Wrap(err, "newSecret - PwEncrypt ")
	}

	tMap := make(map[string]bool)
	for _, tenant := range strings.Split(tenants, ",") {
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

		if err := dbPing(tName, db); err != nil {
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

	config.Secret, err = proto.Marshal(&secret)
	if err != nil {
		return errors.Wrap(err, " newSecret - Marshal")
	}
	viper.Set("secret", config.Secret)
	err = viper.WriteConfig()
	if err != nil {
		return errors.Wrap(err, " newSecret - WriteConfig")
	}

	return nil
}
