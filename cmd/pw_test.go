package cmd

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var ti = []tenantInfo{
	tenantInfo{
		Name: "d01",
	},
	tenantInfo{
		Name: "D02",
	},
	tenantInfo{
		Name: "d03",
	},
}

var config = Config{
	Tenants: ti,
}
var pw1 = "1234"
var pw2 = "5678"
var err error

// ensure set/get of normal byte values is possible.
func Test_pwEncryptDecrypt(t *testing.T) {
	assert := assert.New(t)

	// create secret
	secretkey := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	_, err := rand.Read(secretkey)
	assert.Nil(err)

	// encrypt
	encrypted, err := pwEncrypt([]byte(pw1), secretkey)
	assert.Nil(err)

	// decrypt
	decrypted, err := pwDecrypt(encrypted, secretkey)
	assert.Nil(err)

	assert.Equal(decrypted, pw1)
}

func Test_SecretKey(t *testing.T) {
	assert := assert.New(t)

	sk1, err := getSecretKey()
	assert.Nil(err)

	sk2, err := getSecretKey()
	assert.Nil(err)

	assert.Equal(len(sk1), 32)
	assert.Equal(len(sk2), 32)
	assert.NotEqual(string(sk1), string(sk2), "values should not be equal")
}

func Test_findTenant(t *testing.T) {
	assert := assert.New(t)

	ti1 := config.findTenant("d01")
	assert.Equal(ti1.Name, "d01")

	ti2 := config.findTenant("D01")
	assert.Equal(ti2.Name, "d01")

	ti3 := config.findTenant("D04")
	assert.Equal(ti3, tenantInfo{})
}

func Test_addPwExistingTenantEmptySecret(t *testing.T) {
	assert := assert.New(t)

	config.Secret, err = config.addSecret("D01", []byte(pw1))
	assert.Nil(err)

	sm1, err := config.getSecretMap()
	assert.Nil(err)

	tpw1, err := getPw(sm1, "d01")
	assert.Nil(err)
	assert.Equal(tpw1, pw1)
}

func Test_addPwExistingTenants(t *testing.T) {
	assert := assert.New(t)

	config.Secret, err = config.addSecret("D01", []byte(pw1))
	assert.Nil(err)

	config.Secret, err = config.addSecret("d03,D02", []byte(pw1))
	assert.Nil(err)

	config.Secret, err = config.addSecret("D01", []byte(pw2))
	assert.Nil(err)

	sm, err := config.getSecretMap()
	assert.Nil(err)

	tpw1, err := getPw(sm, "d03")
	assert.Nil(err)
	assert.Equal(tpw1, pw1)
	tpw2, err := getPw(sm, "d02")
	assert.Nil(err)
	assert.Equal(tpw2, pw1)

	tpw3, err := getPw(sm, "d01")
	assert.Nil(err)
	assert.Equal(tpw3, pw2)
}

func Test_addPwNotExistingTenant(t *testing.T) {
	assert := assert.New(t)

	config.Secret, err = config.addSecret("D04", []byte(pw1))
	assert.NotNil(err)
}
