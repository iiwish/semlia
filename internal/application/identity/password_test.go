package identity

import (
	"context"
	"errors"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	"strings"
	"testing"
)

type unusedPasswordRepository struct{ PasswordRepository }

func TestLocalUsername(t *testing.T) {
	for _, value := range []string{"admin", "Admin", "user@example.com"} {
		if _, err := normalizeLocalUsername(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", "bad name", "bad/name"} {
		if _, err := normalizeLocalUsername(value); err == nil {
			t.Fatal("invalid username accepted")
		}
	}
}

func TestPasswordConcurrencyLimitRejectsBeforeRepositoryAccess(t *testing.T) {
	for range cap(passwordWork) {
		passwordWork <- struct{}{}
	}
	defer func() {
		for range cap(passwordWork) {
			<-passwordWork
		}
	}()
	service := &Service{passwords: unusedPasswordRepository{}}
	if _, err := service.PasswordLogin(context.Background(), "user@example.com", "Synthetic test password", "127.0.0.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("unbounded password work: %v", err)
	}
}

func TestPasswordHashPolicyAndVerification(t *testing.T) {
	password := "a long unique passphrase 9287"
	a, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || strings.Contains(a, password) {
		t.Fatal("password hash lacks independent salt")
	}
	if !VerifyPassword(a, password) || VerifyPassword(a, password+"x") {
		t.Fatal("password verification mismatch")
	}
	for _, invalid := range []string{"", "short", "passwordpassword", strings.Repeat(" ", 20), strings.Repeat("x", 1025), "abcdefghijklmn\x00"} {
		if _, err := HashPassword(invalid); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	for _, malformed := range []string{"", "plaintext", "$argon2id$v=19$m=999999999,t=2,p=1$c2FsdA$aGFzaA", strings.Replace(a, "v=19", "v=20", 1), a + "$extra"} {
		if VerifyPassword(malformed, password) {
			t.Fatal("malformed hash accepted")
		}
	}
}

func TestPasswordMinimumSixCharacters(t *testing.T) {
	for _, password := range []string{"a8K2q9", "春夏秋冬山水"} {
		hash, err := HashPassword(password)
		if err != nil || !VerifyPassword(hash, password) {
			t.Fatal("six-character password rejected", err)
		}
	}
	for _, password := range []string{"a8K2q", "春夏秋冬山"} {
		if _, err := HashPassword(password); err == nil {
			t.Fatal("five-character password accepted")
		}
	}
}
