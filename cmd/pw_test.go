package cmd

import (
	"testing"

	"github.com/ulranh/hana_sql_exporter/internal"
)

func Test_pw(t *testing.T) {

	var err error
	// var tenants string
	var secret internal.Secret
	secret.Name = make(map[string][]uint8)

	secretkey, err := internal.GetSecretKey()
	internal.Ok(t, err)

	t1 := "P01,Q01"
	t2 := "T01"
	t3 := "P01"
	pw1 := "123456"
	pw2 := "abcdef"

	secret.Name["secretkey"] = secretkey
	err = newSecret(secret, &t1, []byte(pw1))
	internal.Ok(t, err)

	decPw, err := internal.PwDecrypt(secret.Name["q01"], secretkey)
	internal.Ok(t, err)
	internal.Equals(t, decPw, pw1)
	decPw, err = internal.PwDecrypt(secret.Name["p01"], secretkey)
	internal.Ok(t, err)
	internal.Equals(t, decPw, pw1)

	err = newSecret(secret, &t2, []byte(pw1))
	internal.Ok(t, err)
	decPw, err = internal.PwDecrypt(secret.Name["t01"], secretkey)
	internal.Ok(t, err)
	internal.Equals(t, decPw, pw1)

	err = newSecret(secret, &t3, []byte(pw2))
	internal.Ok(t, err)
	decPw, err = internal.PwDecrypt(secret.Name["p01"], secretkey)
	internal.Ok(t, err)
	internal.Equals(t, decPw, pw2)
}
