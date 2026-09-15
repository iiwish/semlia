package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	"github.com/iiwish/semlia/internal/platform/config"
)

func secureSessionCookies(cfg config.Config) bool {
	for _, origin := range cfg.AllowedOrigins {
		if strings.HasPrefix(origin, "http://") {
			return false
		}
	}
	return true
}

// Passwords enter through a pipe, never arguments, environment or an echoing terminal.
func localPasswordCommand(ctx context.Context, cfg config.Config, args []string, input io.Reader, output io.Writer) error {
	if file, ok := input.(*os.File); ok {
		info, err := file.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice != 0 {
			return errors.New("password requires piped stdin")
		}
	}
	data, err := io.ReadAll(io.LimitReader(input, 1027))
	if err != nil || len(data) > 1026 {
		return errors.New("invalid password input")
	}
	defer clear(data)
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	pool, err := pgstore.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	service, err := identityapp.NewLocalService(pgstore.NewStore(pool), []byte(cfg.SecretKey))
	if err != nil {
		return err
	}
	if args[0] == "reset-local-password" {
		if err := service.ResetLocalPassword(ctx, args[1], password); err != nil {
			return err
		}
		fmt.Fprintln(output, "password reset; all account sessions revoked")
		return nil
	}
	account, err := service.BootstrapLocalAdministrator(ctx, args[1], args[2], args[3], password)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "local administrator: %s\n", account.ID.String())
	return nil
}
