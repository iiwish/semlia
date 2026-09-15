package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"

	domain "github.com/iiwish/semlia/internal/domain/identity"
	"golang.org/x/crypto/argon2"
)

const passwordPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"

func validPassword(password string) bool {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 6 || len(password) > 1024 || strings.IndexFunc(password, unicode.IsControl) >= 0 {
		return false
	}
	weak := strings.ToLower(strings.TrimSpace(password))
	if weak == "" {
		return false
	}
	for _, common := range []string{"passwordpassword", "password123456789", "123456789012345", "qwertyuiopasdfgh", "letmeinletmeinletmein", "correct horse battery staple"} {
		if weak == common {
			return false
		}
	}
	return len(strings.Trim(weak, string([]rune(weak)[0]))) != 0
}

func HashPassword(password string) (string, error) {
	if !validPassword(password) {
		return "", domain.ErrInvalidArgument
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return passwordPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

func VerifyPassword(encoded, password string) bool {
	if len(password) > 1024 || !utf8.ValidString(password) || !strings.HasPrefix(encoded, passwordPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(encoded, passwordPrefix), "$")
	if len(parts) != 2 || len(parts[0]) != 22 || len(parts[1]) != 43 {
		return false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(want) != 32 {
		return false
	}
	key := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return subtle.ConstantTimeCompare(want, key) == 1
}
