package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
)

type PasswordCredential struct {
	AccountID publicid.UserAccountID
	Hash      string
	Version   int64
}

type LocalAccountRecord struct {
	Username, DisplayName, PasswordHash, RoleID string
	WorkspaceID                                 publicid.WorkspaceID
	WorkspaceSlug, WorkspaceName                string
	Actor                                       publicid.PrincipalID
	AuthorizationVersion, RoleVersion           int64
	Bootstrap                                   bool
	Now                                         time.Time
}

type PasswordRepository interface {
	LoadPasswordCredential(context.Context, string) (PasswordCredential, error)
	LoadAccountPassword(context.Context, publicid.UserAccountID) (PasswordCredential, error)
	TakePasswordLoginBudget(context.Context, string, string, time.Time) (bool, error)
	CreatePasswordSession(context.Context, CreateSessionRecord, int64) error
	ReplacePassword(context.Context, publicid.UserAccountID, int64, string, time.Time) error
	CreateLocalAccount(context.Context, LocalAccountRecord) (domain.Account, error)
}

// Bound all password operations across service instances in the process.
var passwordWork = make(chan struct{}, 4)

func normalizeLocalUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "@") {
		return domain.NormalizeEmail(value)
	}
	if len(value) == 0 || len(value) > 64 {
		return "", domain.ErrInvalidArgument
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return "", domain.ErrInvalidArgument
		}
	}
	return value, nil
}

func NewLocalService(repository Repository, rootSecret []byte, options ...Option) (*Service, error) {
	passwords, ok := repository.(PasswordRepository)
	if !ok || len(rootSecret) < 32 {
		return nil, errors.New("password repository and root secret required")
	}
	service, err := newSessionService(repository, nil, rootSecret, options...)
	if err != nil {
		return nil, err
	}
	service.passwords = passwords
	service.dummyHash, err = HashPassword("Unusable random-account placeholder " + hex.EncodeToString(service.csrfKey))
	return service, err
}

func acquirePasswordWork() bool {
	select {
	case passwordWork <- struct{}{}:
		return true
	default:
		return false
	}
}

func (service *Service) PasswordLogin(ctx context.Context, username, password, source string) (LoginComplete, error) {
	if service.passwords == nil {
		return LoginComplete{}, domain.ErrForbidden
	}
	if !acquirePasswordWork() {
		return LoginComplete{}, domain.ErrRateLimited
	}
	defer func() { <-passwordWork }()
	username, normalizeErr := normalizeLocalUsername(username)
	if len(username) > 320 || len(password) > 1024 || len(source) > 256 {
		return LoginComplete{}, domain.ErrUnauthenticated
	}
	allowed, err := service.passwords.TakePasswordLoginBudget(ctx, service.passwordBudgetKey("account:"+username), service.passwordBudgetKey("source:"+source), service.clock().UTC())
	if err != nil {
		return LoginComplete{}, err
	}
	if !allowed {
		return LoginComplete{}, domain.ErrRateLimited
	}
	credential, err := service.passwords.LoadPasswordCredential(ctx, username)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return LoginComplete{}, err
	}
	hash := credential.Hash
	if err != nil || normalizeErr != nil || username == "" {
		hash = service.dummyHash
	}
	valid := VerifyPassword(hash, password)
	if !valid || err != nil || normalizeErr != nil || username == "" {
		return LoginComplete{}, domain.ErrUnauthenticated
	}
	token, err := service.token(32)
	if err != nil {
		return LoginComplete{}, err
	}
	id, err := publicid.NewSessionID()
	if err != nil {
		return LoginComplete{}, err
	}
	now := service.clock().UTC()
	expires := now.Add(service.sessionTTL)
	err = service.passwords.CreatePasswordSession(ctx, CreateSessionRecord{ID: id, AccountID: credential.AccountID, TokenDigest: sha256.Sum256([]byte(token)), CSRFDigest: sha256.Sum256([]byte(service.csrfToken(token))), IdleExpiresAt: now.Add(service.sessionIdleTTL), AbsoluteExpiresAt: expires, Now: now}, credential.Version)
	if err != nil {
		return LoginComplete{}, err
	}
	return LoginComplete{SessionToken: token, ExpiresAt: expires, ReturnTo: "/"}, nil
}

func (service *Service) ChangePassword(ctx context.Context, account publicid.UserAccountID, current, replacement, source string) error {
	if service.passwords == nil {
		return domain.ErrForbidden
	}
	if !acquirePasswordWork() {
		return domain.ErrRateLimited
	}
	defer func() { <-passwordWork }()
	if len(source) > 256 || len(current) > 1024 {
		return domain.ErrInvalidArgument
	}
	allowed, err := service.passwords.TakePasswordLoginBudget(ctx, service.passwordBudgetKey("change:"+account.String()), service.passwordBudgetKey("source:"+source), service.clock().UTC())
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrRateLimited
	}
	credential, err := service.passwords.LoadAccountPassword(ctx, account)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if err != nil || !VerifyPassword(credential.Hash, current) {
		return domain.ErrUnauthenticated
	}
	hash, err := HashPassword(replacement)
	if err != nil {
		return err
	}
	return service.passwords.ReplacePassword(ctx, account, credential.Version, hash, service.clock().UTC())
}

func (service *Service) passwordBudgetKey(value string) string {
	m := hmac.New(sha256.New, service.csrfKey)
	_, _ = m.Write([]byte(value))
	return hex.EncodeToString(m.Sum(nil))
}

func (service *Service) HasLocalPassword(ctx context.Context, account publicid.UserAccountID) (bool, error) {
	if service.passwords == nil {
		return false, nil
	}
	_, err := service.passwords.LoadAccountPassword(ctx, account)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (service *Service) CreatePasswordMember(ctx context.Context, workspace publicid.WorkspaceID, actor publicid.PrincipalID, username, displayName, password, roleID, traceID string) (domain.Account, error) {
	if service.passwords == nil {
		return domain.Account{}, domain.ErrForbidden
	}
	decision, err := service.authorize(ctx, workspace, actor.String(), authorization.ActionMemberManage, traceID)
	if err != nil {
		return domain.Account{}, err
	}
	preparation, err := service.authorizer.PrepareInvitationGrant(ctx, authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: actor.String(), TraceID: traceID}, roleID)
	if err != nil {
		return domain.Account{}, err
	}
	if decision.AuthorizationVersion != preparation.AuthorizationVersion {
		return domain.Account{}, authorization.ErrVersionConflict
	}
	return service.createPasswordAccount(ctx, LocalAccountRecord{WorkspaceID: workspace, Actor: actor, Username: username, DisplayName: displayName, RoleID: roleID, RoleVersion: preparation.RoleVersion, AuthorizationVersion: decision.AuthorizationVersion}, password)
}

func (service *Service) BootstrapLocalAdministrator(ctx context.Context, slug, name, username, password string) (domain.Account, error) {
	return service.createPasswordAccount(ctx, LocalAccountRecord{Username: username, DisplayName: username, WorkspaceSlug: strings.TrimSpace(slug), WorkspaceName: strings.TrimSpace(name), RoleID: "workspace_admin", RoleVersion: 1, Bootstrap: true}, password)
}

func (service *Service) createPasswordAccount(ctx context.Context, record LocalAccountRecord, password string) (domain.Account, error) {
	if service.passwords == nil {
		return domain.Account{}, domain.ErrForbidden
	}
	username, err := normalizeLocalUsername(record.Username)
	if err != nil || username == "" || strings.TrimSpace(record.DisplayName) == "" || len(record.DisplayName) > 256 || (record.Bootstrap && (record.WorkspaceSlug == "" || record.WorkspaceName == "")) {
		return domain.Account{}, domain.ErrInvalidArgument
	}
	if !acquirePasswordWork() {
		return domain.Account{}, domain.ErrRateLimited
	}
	defer func() { <-passwordWork }()
	hash, err := HashPassword(password)
	if err != nil {
		return domain.Account{}, err
	}
	record.Username = username
	record.PasswordHash = hash
	record.Now = service.clock().UTC()
	return service.passwords.CreateLocalAccount(ctx, record)
}

func (service *Service) ResetLocalPassword(ctx context.Context, username, password string) error {
	if service.passwords == nil {
		return domain.ErrForbidden
	}
	username, err := normalizeLocalUsername(username)
	if err != nil || username == "" {
		return domain.ErrInvalidArgument
	}
	if !acquirePasswordWork() {
		return domain.ErrRateLimited
	}
	defer func() { <-passwordWork }()
	credential, err := service.passwords.LoadPasswordCredential(ctx, username)
	if err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return service.passwords.ReplacePassword(ctx, credential.AccountID, credential.Version, hash, service.clock().UTC())
}
