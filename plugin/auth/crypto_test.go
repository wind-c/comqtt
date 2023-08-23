package auth

import (
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestBcrypt(t *testing.T) {
	pwd := "123456"
	hashed1 := Bcrypt(pwd)
	hashed2 := Bcrypt(pwd)
	println(hashed1)
	require.NotEqual(t, hashed1, hashed2)
	err := bcrypt.CompareHashAndPassword([]byte(hashed1), []byte(pwd))
	require.NoError(t, err)
	err = bcrypt.CompareHashAndPassword([]byte(hashed2), []byte(pwd))
	require.NoError(t, err)
}
