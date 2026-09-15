package identity_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var databaseURL string

func TestMain(testingMain *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("semlia_identity_test"), tcpostgres.WithUsername("semlia"), tcpostgres.WithPassword("integration-test-only"), tcpostgres.BasicWaitStrategies())
	if err != nil {
		fmt.Fprintln(os.Stderr, "start identity PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "migrate identity PostgreSQL: failed")
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

type providerStub struct {
	nonce  string
	claims domain.OIDCClaims
}

type interleavingAuthorizer struct {
	delegate       *authorizationapp.Service
	onMemberManage func(authorization.Decision)
	once           sync.Once
}

func (authorizer *interleavingAuthorizer) Evaluate(ctx context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	decision, err := authorizer.delegate.Evaluate(ctx, request)
	if err == nil && decision.Allowed && request.Action == authorization.ActionMemberManage && authorizer.onMemberManage != nil {
		authorizer.once.Do(func() { authorizer.onMemberManage(decision) })
	}
	return decision, err
}

func (authorizer *interleavingAuthorizer) PrepareInvitationGrant(ctx context.Context, request authorizationapp.AccessRequest, roleID string) (authorizationapp.InvitationGrantPreparation, error) {
	return authorizer.delegate.PrepareInvitationGrant(ctx, request, roleID)
}

func (authorizer *interleavingAuthorizer) RecordPolicyDenial(ctx context.Context, denial authorizationapp.PolicyDenial) error {
	return authorizer.delegate.RecordPolicyDenial(ctx, denial)
}

func (*providerStub) Issuer() string { return "https://identity.example" }

func (provider *providerStub) AuthorizationURL(_, nonce, _ string) string {
	provider.nonce = nonce
	return "https://identity.example/authorize"
}

func (provider *providerStub) ExchangeAndVerify(context.Context, string, string) (domain.OIDCClaims, error) {
	claims := provider.claims
	claims.Nonce = provider.nonce
	return claims, nil
}

func TestBootstrapLoginSessionRestartRevocationAndFinalAdmin(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	provider := &providerStub{claims: domain.OIDCClaims{Issuer: "https://identity.example", Subject: "founder-subject", DisplayName: "Founder", Email: "founder@example.com", EmailVerified: true}}
	rootSecret := bytes.Repeat([]byte{3}, 32)
	service, err := identityapp.NewService(store, provider, rootSecret)
	if err != nil {
		t.Fatal(err)
	}

	firstBootstrap, err := service.BootstrapAdministrator(ctx, "alpha", "Alpha", "Founder@example.com")
	if err != nil {
		t.Fatal(err)
	}
	secondBootstrap, err := service.BootstrapAdministrator(ctx, "alpha", "Alpha", "founder@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if firstBootstrap.Invitation.ID != secondBootstrap.Invitation.ID || firstBootstrap.AcceptanceToken != secondBootstrap.AcceptanceToken {
		t.Fatal("bootstrap was not idempotent")
	}
	var invitations int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM workspace_invitations").Scan(&invitations); err != nil || invitations != 1 {
		t.Fatalf("bootstrap invitations = %d, err = %v", invitations, err)
	}

	started, err := service.BeginLogin(ctx, "/catalog")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.CompleteLogin(ctx, started.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(ctx, started.State, "code"); !errors.Is(err, domain.ErrOIDCTransaction) {
		t.Fatalf("replayed login = %v", err)
	}

	authenticated, err := service.Authenticate(ctx, completed.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(authenticated.Session.Memberships) != 1 || authenticated.Session.Memberships[0].RoleIDs[0] != "workspace_admin" {
		t.Fatalf("memberships = %+v", authenticated.Session.Memberships)
	}
	restarted, err := identityapp.NewService(pgstore.NewStore(pool), provider, rootSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Authenticate(ctx, completed.SessionToken); err != nil {
		t.Fatalf("session did not survive service restart: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state)
		SELECT semlia_seed_uuidv7('session_perf_asset', $1::text || ':' || series::text, CURRENT_TIMESTAMP),
		       $1::uuid, 'session_perf', 'metric_' || lpad(series::text, 5, '0'), 'metric', 'active'
		FROM generate_series(1, 10000) AS series
	`, authenticated.Session.Memberships[0].WorkspaceID.UUID()); err != nil {
		t.Fatalf("seed 10,000-asset session reference stack: %v", err)
	}
	samples := make([]time.Duration, 0, 25)
	for range 25 {
		startedAt := time.Now()
		if _, err := restarted.Authenticate(ctx, completed.SessionToken); err != nil {
			t.Fatalf("benchmark session validation: %v", err)
		}
		samples = append(samples, time.Since(startedAt))
	}
	sort.Slice(samples, func(left, right int) bool { return samples[left] < samples[right] })
	p95 := samples[23]
	t.Logf("session validation benchmark: host=%s/%s assets=10000 samples=25 p50=%s p95=%s", runtime.GOOS, runtime.GOARCH, samples[12], p95)
	if p95 >= time.Second {
		t.Fatalf("session validation p95 = %s, budget < 1s", p95)
	}

	membership := authenticated.Session.Memberships[0]
	if _, err := store.SetMembershipStatus(ctx, membership.WorkspaceID, membership.PrincipalID, membership.ID, domain.MembershipSuspended, membership.AuthorizationVersion, time.Now().UTC()); !errors.Is(err, domain.ErrFinalAdministrator) {
		t.Fatalf("final administrator suspension = %v", err)
	}
	if err := restarted.Logout(ctx, completed.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Authenticate(ctx, completed.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked session authentication = %v", err)
	}

	var tokenDigest []byte
	if err := pool.QueryRow(ctx, "SELECT token_digest FROM sessions LIMIT 1").Scan(&tokenDigest); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(tokenDigest, []byte(completed.SessionToken)) || len(tokenDigest) != 32 {
		t.Fatal("session token was not stored as a fixed-length digest")
	}
	var auditPayloads string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(string_agg(payload::text, ''), '') FROM audit_events WHERE event_type LIKE 'identity.%'`).Scan(&auditPayloads); err != nil {
		t.Fatal(err)
	}
	if auditPayloads == "" || bytes.Contains([]byte(auditPayloads), []byte(completed.SessionToken)) || bytes.Contains([]byte(auditPayloads), []byte("founder@example.com")) {
		t.Fatalf("identity audit payloads are missing or contain sensitive material: %s", auditPayloads)
	}
}

func TestInvitationFreezesRoleVersionAndCannotBypassGrantorCeiling(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	authorizer := authorizationapp.NewService(store, authorizationapp.ClockFunc(time.Now))
	provider := &providerStub{claims: domain.OIDCClaims{Issuer: "https://identity.example", Subject: "founder", DisplayName: "Founder", Email: "founder@example.com", EmailVerified: true}}
	service, err := identityapp.NewService(store, provider, bytes.Repeat([]byte{5}, 32), identityapp.WithAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BootstrapAdministrator(ctx, "invite-policy", "Invite Policy", "founder@example.com"); err != nil {
		t.Fatal(err)
	}
	started, err := service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	login, err := service.CompleteLogin(ctx, started.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := service.Authenticate(ctx, login.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	membership := authenticated.Session.Memberships[0]
	access := authorizationapp.AccessRequest{WorkspaceID: membership.WorkspaceID, PrincipalRef: membership.PrincipalID.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}
	role, _, err := authorizer.CreateCustomRole(ctx, authorizationapp.CreateCustomRoleRequest{
		AccessRequest: access, Name: "Invited reader", Description: "Invitation version policy.",
		Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Email: "reader@example.com", RoleID: role.ID,
	}, access.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Invitation.RoleVersion != 1 {
		t.Fatalf("frozen invitation role version = %d", issued.Invitation.RoleVersion)
	}
	if _, _, err := authorizer.UpdateCustomRole(ctx, authorizationapp.UpdateCustomRoleRequest{
		AccessRequest: access, RoleID: role.ID, ExpectedVersion: 1, Name: role.Name,
		Description: "Invitation version policy v2.", Actions: []authorization.Action{authorization.ActionEvidenceRead},
	}); err != nil {
		t.Fatal(err)
	}
	provider.claims = domain.OIDCClaims{Issuer: "https://identity.example", Subject: "reader", DisplayName: "Reader", Email: "reader@example.com", EmailVerified: true}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(ctx, started.State, "code"); !errors.Is(err, authorization.ErrVersionConflict) {
		t.Fatalf("stale invitation acceptance error = %v", err)
	}
	var deniedPayload string
	if err := pool.QueryRow(ctx, `
		SELECT payload::text FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'authorization.policy_denied'
		ORDER BY created_at DESC LIMIT 1`, membership.WorkspaceID.UUID()).Scan(&deniedPayload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deniedPayload, "STALE_INVITATION_ROLE") || strings.Contains(deniedPayload, "reader@example.com") {
		t.Fatalf("invitation denial audit payload = %s", deniedPayload)
	}

	securityID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePrincipal(ctx, authorization.Principal{
		ID: securityID, WorkspaceID: membership.WorkspaceID, Kind: authorization.PrincipalHuman,
		DisplayName: "Security Admin", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	bindingID, _ := identity.NewBindingID()
	if _, err := store.CreateRoleBinding(ctx, authorization.RoleBinding{
		ID: bindingID, PrincipalID: securityID, RoleID: "security_admin",
		ScopeType: authorization.ScopeWorkspace, ScopeID: membership.WorkspaceID.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Invite(ctx, membership.WorkspaceID, securityID, identityapp.InvitationInput{
		Email: "ceiling@example.com", RoleID: "consumer_developer",
	}, access.TraceID); !errors.Is(err, authorization.ErrAuthorizationCeiling) {
		t.Fatalf("invitation ceiling error = %v", err)
	}

	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Email: "duties@example.com", RoleID: "reviewer",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	provider.claims = domain.OIDCClaims{Issuer: "https://identity.example", Subject: "duties", DisplayName: "Duties", Email: "duties@example.com", EmailVerified: true}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	dutiesLogin, err := service.CompleteLogin(ctx, started.State, "code")
	if err != nil {
		t.Fatalf("accept reviewer invitation: %v", err)
	}
	dutiesSession, err := service.Authenticate(ctx, dutiesLogin.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	dutiesMembership := dutiesSession.Session.Memberships[0]
	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Email: "duties@example.com", RoleID: "publisher",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(ctx, started.State, "code"); !errors.Is(err, authorization.ErrSeparationOfDuties) {
		t.Fatalf("invitation separation-of-duties error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE workspace_invitations
		SET status = 'revoked', revoked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE workspace_id = $1 AND email = 'duties@example.com' AND status = 'pending'`, membership.WorkspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Subject: "duties", RoleID: "reviewer",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	var expiredBindingID string
	if err := pool.QueryRow(ctx, `
		UPDATE role_bindings
		SET expires_at = granted_at + interval '1 microsecond'
		WHERE workspace_id = $1 AND principal_id = $2 AND role_id = 'reviewer'
		  AND revoked_at IS NULL AND expired_at IS NULL
		RETURNING id::text`, membership.WorkspaceID.UUID(), dutiesMembership.PrincipalID.UUID()).Scan(&expiredBindingID); err != nil {
		t.Fatal(err)
	}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(ctx, started.State, "code"); err != nil {
		t.Fatalf("invitation regrant after logical expiry: %v", err)
	}
	var materializedExpired, activeReviewer int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE id = $1::uuid AND expired_at IS NOT NULL`, expiredBindingID).Scan(&materializedExpired); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_bindings
		WHERE workspace_id = $1 AND principal_id = $2 AND role_id = 'reviewer'
		  AND revoked_at IS NULL AND expired_at IS NULL
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`, membership.WorkspaceID.UUID(), dutiesMembership.PrincipalID.UUID()).Scan(&activeReviewer); err != nil {
		t.Fatal(err)
	}
	if materializedExpired != 1 || activeReviewer != 1 {
		t.Fatalf("logical-expiry regrant state: materialized=%d active=%d", materializedExpired, activeReviewer)
	}
	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Subject: "duties", RoleID: "reviewer",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(ctx, started.State, "code"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate active invitation grant error = %v", err)
	}
	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Subject: "dual-duty", RoleID: "reviewer",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Email: "dual-duty@example.com", RoleID: "publisher",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	provider.claims = domain.OIDCClaims{Issuer: "https://identity.example", Subject: "dual-duty", DisplayName: "Dual Duty", Email: "dual-duty@example.com", EmailVerified: true}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(ctx, started.State, "code"); !errors.Is(err, authorization.ErrSeparationOfDuties) {
		t.Fatalf("same-transaction invitation separation error = %v", err)
	}

	if _, err := service.Invite(ctx, membership.WorkspaceID, membership.PrincipalID, identityapp.InvitationInput{
		Email: "admin2@example.com", RoleID: "workspace_admin",
	}, access.TraceID); err != nil {
		t.Fatal(err)
	}
	provider.claims = domain.OIDCClaims{Issuer: "https://identity.example", Subject: "admin2", DisplayName: "Admin Two", Email: "admin2@example.com", EmailVerified: true}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	adminLogin, err := service.CompleteLogin(ctx, started.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := service.Authenticate(ctx, adminLogin.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	adminMembership := adminSession.Session.Memberships[0]
	bindings, err := store.ListAuthorizationRoleBindings(ctx, membership.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var adminBinding authorization.RoleBinding
	for _, binding := range bindings {
		if binding.PrincipalID == adminMembership.PrincipalID && binding.RoleID == "workspace_admin" {
			adminBinding = binding
		}
	}
	if adminBinding.ID.IsZero() {
		t.Fatal("second administrator binding is missing")
	}
	errorsByOperation := make(chan error, 2)
	go func() {
		_, updateErr := service.SetMembershipStatus(ctx, membership.WorkspaceID, membership.PrincipalID, membership.ID, domain.MembershipSuspended, access.TraceID)
		errorsByOperation <- updateErr
	}()
	go func() {
		_, _, revokeErr := authorizer.RevokeBinding(ctx, authorizationapp.RevokeRoleBindingRequest{
			AccessRequest: access, BindingID: adminBinding.ID, ExpectedVersion: adminBinding.Version, Reason: "concurrent final-admin proof",
		})
		errorsByOperation <- revokeErr
	}()
	firstErr, secondErr := <-errorsByOperation, <-errorsByOperation
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatalf("concurrent admin mutations must serialize to one success: first=%v second=%v", firstErr, secondErr)
	}
	var activeAdmins int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM role_bindings AS binding
		JOIN principals AS principal ON principal.id = binding.principal_id AND principal.workspace_id = binding.workspace_id
		JOIN workspace_memberships AS membership ON membership.principal_id = binding.principal_id AND membership.workspace_id = binding.workspace_id
		WHERE binding.workspace_id = $1 AND binding.role_id = 'workspace_admin'
		  AND binding.revoked_at IS NULL AND binding.expired_at IS NULL
		  AND (binding.expires_at IS NULL OR binding.expires_at > CURRENT_TIMESTAMP)
		  AND principal.status = 'active' AND membership.status = 'active'`, membership.WorkspaceID.UUID()).Scan(&activeAdmins); err != nil {
		t.Fatal(err)
	}
	if activeAdmins < 1 {
		t.Fatalf("active workspace administrators after concurrency = %d", activeAdmins)
	}
}

func TestMembershipMutationRejectsRevokedMemberManageDecisionVersion(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	authorizer := authorizationapp.NewService(store, authorizationapp.ClockFunc(time.Now))
	provider := &providerStub{claims: domain.OIDCClaims{Issuer: "https://identity.example", Subject: "race-founder", DisplayName: "Founder", Email: "race-founder@example.com", EmailVerified: true}}
	service, err := identityapp.NewService(store, provider, bytes.Repeat([]byte{6}, 32), identityapp.WithAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BootstrapAdministrator(ctx, "identity-authz-race", "Identity Authz Race", "race-founder@example.com"); err != nil {
		t.Fatal(err)
	}
	started, err := service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	founderLogin, err := service.CompleteLogin(ctx, started.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	founderSession, err := service.Authenticate(ctx, founderLogin.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	founder := founderSession.Session.Memberships[0]
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	if _, err := service.Invite(ctx, founder.WorkspaceID, founder.PrincipalID, identityapp.InvitationInput{
		Email: "race-admin2@example.com", RoleID: "workspace_admin",
	}, traceID); err != nil {
		t.Fatal(err)
	}
	provider.claims = domain.OIDCClaims{Issuer: "https://identity.example", Subject: "race-admin2", DisplayName: "Admin Two", Email: "race-admin2@example.com", EmailVerified: true}
	started, err = service.BeginLogin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	adminLogin, err := service.CompleteLogin(ctx, started.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := service.Authenticate(ctx, adminLogin.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	target := adminSession.Session.Memberships[0]
	bindings, err := store.ListAuthorizationRoleBindings(ctx, founder.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var founderBinding authorization.RoleBinding
	for _, binding := range bindings {
		if binding.PrincipalID == founder.PrincipalID && binding.RoleID == "workspace_admin" {
			founderBinding = binding
		}
	}
	if founderBinding.ID.IsZero() {
		t.Fatal("founder administrator binding is missing")
	}
	var revokeErr error
	interleaving := &interleavingAuthorizer{delegate: authorizer}
	interleaving.onMemberManage = func(authorization.Decision) {
		_, _, revokeErr = authorizer.RevokeBinding(ctx, authorizationapp.RevokeRoleBindingRequest{
			AccessRequest: authorizationapp.AccessRequest{WorkspaceID: founder.WorkspaceID, PrincipalRef: founder.PrincipalID.String(), TraceID: traceID},
			BindingID:     founderBinding.ID, ExpectedVersion: founderBinding.Version, Reason: "interleave member.manage revocation",
		})
	}
	raceService, err := identityapp.NewService(store, provider, bytes.Repeat([]byte{6}, 32), identityapp.WithAuthorizer(interleaving))
	if err != nil {
		t.Fatal(err)
	}
	_, err = raceService.SetMembershipStatus(ctx, founder.WorkspaceID, founder.PrincipalID, target.ID, domain.MembershipSuspended, traceID)
	if revokeErr != nil {
		t.Fatalf("interleaved member.manage revocation: %v", revokeErr)
	}
	if !errors.Is(err, authorization.ErrVersionConflict) {
		t.Fatalf("membership mutation after member.manage revocation error = %v", err)
	}
	refreshed, err := store.ListWorkspaceMembers(ctx, founder.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range refreshed {
		if item.ID == target.ID && item.Status != domain.MembershipActive {
			t.Fatalf("stale member.manage decision changed target membership: %+v", item)
		}
	}
}

func TestInvitationCreationRejectsInterleavedMemberManageRevocation(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	workspaceID, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id, slug, display_name) VALUES ($1, 'invite-member-race', 'Invite Member Race')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	authorizer := authorizationapp.NewService(store, authorizationapp.ClockFunc(time.Now))
	admin, err := store.LoadDefaultPrincipal(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	access := authorizationapp.AccessRequest{WorkspaceID: workspaceID, PrincipalRef: admin.ID.String(), TraceID: traceID}
	createRole := func(name string, actions []authorization.Action) authorization.Role {
		t.Helper()
		role, _, createErr := authorizer.CreateCustomRole(ctx, authorizationapp.CreateCustomRoleRequest{
			AccessRequest: access, Name: name, Description: name + " role.", Actions: actions,
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return role
	}
	memberRole := createRole("Member manager", []authorization.Action{authorization.ActionMemberManage})
	assignerRole := createRole("Scoped assigner", []authorization.Action{authorization.ActionRoleAssign, authorization.ActionAssetRead})
	targetRole := createRole("Invited asset reader", []authorization.Action{authorization.ActionAssetRead})
	actorID, _ := identity.NewPrincipalID()
	if _, err := store.CreatePrincipal(ctx, authorization.Principal{
		ID: actorID, WorkspaceID: workspaceID, Kind: authorization.PrincipalHuman,
		DisplayName: "Invitation Manager", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	createBinding := func(role authorization.Role) authorization.RoleBinding {
		t.Helper()
		binding, _, createErr := authorizer.CreateBinding(ctx, authorizationapp.CreateRoleBindingRequest{
			AccessRequest: access, PrincipalID: actorID, RoleID: role.ID, ExpectedRoleVersion: role.Version,
			ScopeType: authorization.ScopeWorkspace, ScopeID: workspaceID.UUID(),
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return binding
	}
	memberBinding := createBinding(memberRole)
	_ = createBinding(assignerRole)
	interleaving := &interleavingAuthorizer{delegate: authorizer}
	var revokeErr error
	interleaving.onMemberManage = func(authorization.Decision) {
		_, _, revokeErr = authorizer.RevokeBinding(ctx, authorizationapp.RevokeRoleBindingRequest{
			AccessRequest: access, BindingID: memberBinding.ID, ExpectedVersion: memberBinding.Version,
			Reason: "interleave invitation member.manage revocation",
		})
	}
	provider := &providerStub{}
	service, err := identityapp.NewService(store, provider, bytes.Repeat([]byte{7}, 32), identityapp.WithAuthorizer(interleaving))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Invite(ctx, workspaceID, actorID, identityapp.InvitationInput{
		Email: "race-invite@example.com", RoleID: targetRole.ID,
	}, traceID)
	if revokeErr != nil {
		t.Fatalf("interleaved member.manage revocation: %v", revokeErr)
	}
	if !errors.Is(err, authorization.ErrVersionConflict) {
		t.Fatalf("invitation after member.manage revocation error = %v", err)
	}
	var invitations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspace_invitations WHERE workspace_id = $1`, workspaceID.UUID()).Scan(&invitations); err != nil {
		t.Fatal(err)
	}
	if invitations != 0 {
		t.Fatalf("stale member.manage decision created %d invitations", invitations)
	}
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
