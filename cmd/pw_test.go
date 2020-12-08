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
func Test_PwEncryptDecrypt(t *testing.T) {

	// create secret
	secretkey := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	_, err := rand.Read(secretkey)
	assert.Nil(t, err)

	// encrypt
	encrypted, err := pwEncrypt([]byte(pw1), secretkey)
	assert.Nil(t, err)

	// decrypt
	decrypted, err := pwDecrypt(encrypted, secretkey)
	assert.Nil(t, err)

	assert.Equal(t, decrypted, pw1)
}

func Test_SecretKey(t *testing.T) {
	sk1, err := getSecretKey()
	assert.Nil(t, err)

	sk2, err := getSecretKey()
	assert.Nil(t, err)

	assert.Equal(t, len(sk1), 32)
	assert.Equal(t, len(sk2), 32)
	assert.NotEqual(t, string(sk1), string(sk2), "values should not be equal")
}

func Test_FindTenant(t *testing.T) {
	ti1 := config.findTenant("d01")
	assert.Equal(t, ti1.Name, "d01")

	ti2 := config.findTenant("D01")
	assert.Equal(t, ti2.Name, "d01")

	ti3 := config.findTenant("D04")
	assert.Equal(t, ti3, tenantInfo{})
}

func Test_AddPwExistingTenantEmptySecret(t *testing.T) {

	config.Secret, err = config.addSecret("D01", []byte(pw1))
	assert.Nil(t, err)

	sm1, err := config.getSecretMap()
	assert.Nil(t, err)

	tpw1, err := getPw(sm1, "d01")
	assert.Nil(t, err)
	assert.Equal(t, tpw1, pw1)
}

func Test_AddPwExistingTenants(t *testing.T) {

	config.Secret, err = config.addSecret("D01", []byte(pw1))
	assert.Nil(t, err)

	config.Secret, err = config.addSecret("d03,D02", []byte(pw1))
	assert.Nil(t, err)

	config.Secret, err = config.addSecret("D01", []byte(pw2))
	assert.Nil(t, err)

	sm, err := config.getSecretMap()
	assert.Nil(t, err)

	tpw1, err := getPw(sm, "d03")
	assert.Nil(t, err)
	assert.Equal(t, tpw1, pw1)
	tpw2, err := getPw(sm, "d02")
	assert.Nil(t, err)
	assert.Equal(t, tpw2, pw1)

	tpw3, err := getPw(sm, "d01")
	assert.Nil(t, err)
	assert.Equal(t, tpw3, pw2)
}

func Test_AddPwNotExistingTenant(t *testing.T) {

	config.Secret, err = config.addSecret("D04", []byte(pw1))
	assert.NotNil(t, err)
}
