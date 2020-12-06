package cmd

import (
	"math/rand"
	"testing"
	"time"

	"github.com/ulranh/hana_sql_exporter/internal"
)

// ensure set/get of normal byte values is possible.
func Test_PwEncryptDecrypt(t *testing.T) {

	// create secret
	secretkey := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	_, err := rand.Read(secretkey)
	internal.Ok(t, err)

	// encrypt
	pw := "123456"
	encrypted, err := pwEncrypt([]byte(pw), secretkey)
	internal.Ok(t, err)

	// decrypt
	decrypted, err := pwDecrypt(encrypted, secretkey)
	internal.Ok(t, err)

	internal.Equals(t, decrypted, pw)
}

func Test_SecretKey(t *testing.T) {
	sk1, err := getSecretKey()
	internal.Ok(t, err)

	sk2, err := getSecretKey()
	internal.Ok(t, err)

	internal.Equals(t, len(sk1), 32)
	internal.Equals(t, len(sk2), 32)
	internal.Assert(t, string(sk1) != string(sk2), "values should not be equal")
}
