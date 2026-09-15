package identity_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	app "github.com/iiwish/semlia/internal/application/identity"
	domain "github.com/iiwish/semlia/internal/domain/identity"
)

type pausedPasswordStore struct {
	*pgstore.Store
	verified chan struct{}
	resume   chan struct{}
}

func (store *pausedPasswordStore) CreatePasswordSession(ctx context.Context, record app.CreateSessionRecord, version int64) error {
	close(store.verified)
	select {
	case <-store.resume:
		return store.Store.CreatePasswordSession(ctx, record, version)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPasswordResetFencesAnAlreadyVerifiedConcurrentLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := &pausedPasswordStore{Store: pgstore.NewStore(pool), verified: make(chan struct{}), resume: make(chan struct{})}
	loginService, err := app.NewLocalService(store, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	resetService, err := app.NewLocalService(store.Store, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	const username = "race-password@example.com"
	const password = "Synthetic concurrent password 8517"
	if _, err := resetService.BootstrapLocalAdministrator(ctx, "password-race", "Password race", username, password); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := loginService.PasswordLogin(ctx, username, password, "127.0.0.9"); result <- err }()
	select {
	case <-store.verified:
	case <-ctx.Done():
		t.Fatal("login never reached the session fence")
	}
	if err := resetService.ResetLocalPassword(ctx, username, "Synthetic replacement password 7193"); err != nil {
		close(store.resume)
		t.Fatal(err)
	}
	close(store.resume)
	if err := <-result; !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("stale concurrent login admitted: %v", err)
	}
}
