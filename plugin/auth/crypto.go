package auth

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

func CompareHash(hashed, plain, key string, ht HashType) bool {
	var tmp string
	switch ht {
	case HashBcrypt:
		if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)); err != nil {
			return false
		} else {
			return true
		}
	case HashNone:
		tmp = plain
	case HashMd5:
		tmp = Md5(plain)
	case HashSha1:
		tmp = Sha1(plain)
	case HashSha256:
		tmp = Sha256(plain)
	case HashSha512:
		tmp = Sha512(plain)
	case HashHmacSha1:
		tmp = HmacSha1(plain, key)
	case HashHmacSha256:
		tmp = HmacSha256(plain, key)
	case HashHmacSha512:
		tmp = HmacSha512(plain, key)
	}

	if tmp == hashed {
		return true
	}

	return false
}

func Bcrypt(src string) string {
	hashed, err := bcrypt.GenerateFromPassword([]byte(src), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hashed)
}

func Md5(src string) string {
	h := md5.New()
	h.Write([]byte(src))
	return hex.EncodeToString(h.Sum(nil))
}

func Sha1(src string) string {
	h := sha1.New()
	h.Write([]byte(src))
	return hex.EncodeToString(h.Sum(nil))
}

func Sha256(src string) string {
	h := sha256.New()
	h.Write([]byte(src))
	return hex.EncodeToString(h.Sum(nil))
}

func Sha512(src string) string {
	h := sha512.New()
	h.Write([]byte(src))
	return hex.EncodeToString(h.Sum(nil))
}

func Base64(src string) string {
	return string(base64.StdEncoding.EncodeToString([]byte(src)))
}

func HmacSha1(src, key string) string {
	m := hmac.New(sha1.New, []byte(key))
	m.Write([]byte(src))
	return hex.EncodeToString(m.Sum(nil))
}

func HmacSha256(src, key string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(src))
	return hex.EncodeToString(m.Sum(nil))
}

func HmacSha512(src, key string) string {
	m := hmac.New(sha512.New, []byte(key))
	m.Write([]byte(src))
	return hex.EncodeToString(m.Sum(nil))
}
