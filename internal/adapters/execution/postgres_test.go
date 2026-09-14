package execution

import (
	"context"
	"encoding/json"
	"testing"

	d "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestDeploymentDSNAuthorityAndTLSFailBeforeConnection(t *testing.T) {
	w, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	config, _ := json.Marshal([]SourceReference{{WorkspaceID: w.String(), SourceID: source.String(), DSNEnv: "SEMLIA_EXECUTION_DSN_TEST"}})
	for _, dsn := range []string{
		"host=approved,other port=5432 dbname=warehouse user=reader sslmode=require",
		"host=approved port=5432 dbname=warehouse user=reader sslmode=prefer",
		"host=approved port=5432 dbname=warehouse user=reader sslmode=allow",
		"host=other port=5432 dbname=warehouse user=reader sslmode=disable",
		"host=approved port=5432 dbname=other user=reader sslmode=disable",
		"host=approved port=5433 dbname=warehouse user=reader sslmode=disable",
	} {
		t.Run(dsn, func(t *testing.T) {
			p, err := NewPostgresWithLookup(string(config), func(string) (string, bool) { return dsn, true })
			if err != nil {
				t.Fatal(err)
			}
			p.WithLocalPlaintext()
			out := p.Execute(context.Background(), w, d.Compiled{SourceID: source.UUID(), SourceLocator: "approved:5432/warehouse"}, d.DefaultLimits())
			if out.ErrorCode != "EXECUTION_SOURCE_MISMATCH" {
				t.Fatalf("unsafe authority attempted connection: %s", out.ErrorCode)
			}
		})
	}
	p, _ := NewPostgresWithLookup(string(config), func(string) (string, bool) { return "host=approved dbname=warehouse user=reader sslmode=disable", true })
	if out := p.Execute(context.Background(), w, d.Compiled{SourceID: source.UUID(), SourceLocator: "approved:5432/warehouse"}, d.DefaultLimits()); out.ErrorCode != "EXECUTION_SOURCE_MISMATCH" {
		t.Fatal("production plaintext accepted")
	}
}
