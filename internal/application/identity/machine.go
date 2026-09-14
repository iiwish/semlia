package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	id "github.com/iiwish/semlia/pkg/identity"
)

type MachineRepository interface {
	SaveClientCredential(context.Context, domain.ClientCredential, int64) error
	LoadClientCredential(context.Context, id.ClientCredentialID) (domain.ClientCredential, error)
	ListClientCredentials(context.Context, id.WorkspaceID) ([]domain.ClientCredential, error)
	RevokeClientCredential(context.Context, id.WorkspaceID, id.ClientCredentialID, id.PrincipalID, time.Time, int64) error
	TouchClientCredential(context.Context, id.ClientCredentialID, time.Time) error
	GetConsumer(context.Context, id.WorkspaceID, id.ConsumerID) (dist.Consumer, error)
	GetConsumerBinding(context.Context, id.WorkspaceID, id.ConsumerBindingID) (dist.ConsumerBinding, error)
	LoadPrincipal(context.Context, id.WorkspaceID, id.PrincipalID) (auth.Principal, error)
	CreateMachinePrincipal(context.Context, auth.Principal, int64) error
}
type MachineService struct {
	repository MachineRepository
	authorizer *authapp.Service
	now        func() time.Time
}

func NewMachineService(repository MachineRepository, authorizer *authapp.Service, now func() time.Time) *MachineService {
	return &MachineService{repository, authorizer, now}
}

func newMachineToken(prefix string) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return prefix + "." + base64.RawURLEncoding.EncodeToString(secret), nil
}
func machineDigest(token string) [32]byte { return sha256.Sum256([]byte(token)) }
func verifyMachineToken(token string, digest [32]byte) bool {
	candidate := machineDigest(token)
	return subtle.ConstantTimeCompare(candidate[:], digest[:]) == 1
}

func (s *MachineService) authorize(ctx context.Context, workspace id.WorkspaceID, actor, trace string, action auth.Action) (auth.Decision, error) {
	d, err := s.authorizer.Evaluate(ctx, authapp.EvaluationRequest{WorkspaceID: workspace, PrincipalRef: actor, TraceID: trace, Action: action, Resource: auth.Resource{Type: auth.ScopeWorkspace, ID: workspace.UUID()}})
	if err != nil {
		return d, err
	}
	if !d.Allowed {
		return d, &auth.DenialError{Decision: d}
	}
	return d, nil
}

func (s *MachineService) CreatePrincipal(ctx context.Context, workspace id.WorkspaceID, actor, trace, name string) (auth.Principal, error) {
	d, err := s.authorize(ctx, workspace, actor, trace, auth.ActionMemberManage)
	if err != nil {
		return auth.Principal{}, err
	}
	principalID, err := id.NewPrincipalID()
	if err != nil {
		return auth.Principal{}, err
	}
	p := auth.Principal{ID: principalID, WorkspaceID: workspace, Kind: auth.PrincipalAgent, DisplayName: strings.TrimSpace(name), OwnerPrincipalID: &d.PrincipalID, Status: auth.PrincipalActive, CreatedAt: s.now().UTC()}
	if len(p.DisplayName) > 160 || p.Validate() != nil {
		return p, domain.ErrInvalidArgument
	}
	return p, s.repository.CreateMachinePrincipal(ctx, p, d.AuthorizationVersion)
}

func (s *MachineService) List(ctx context.Context, workspace id.WorkspaceID, actor, trace string) ([]domain.ClientCredential, error) {
	if _, err := s.authorize(ctx, workspace, actor, trace, auth.ActionBindingRead); err != nil {
		return nil, err
	}
	return s.repository.ListClientCredentials(ctx, workspace)
}

type IssueMachineCredential struct {
	WorkspaceID    id.WorkspaceID         `json:"-"`
	Actor          string                 `json:"-"`
	TraceID        string                 `json:"-"`
	PrincipalID    id.PrincipalID         `json:"principalId"`
	ConsumerID     id.ConsumerID          `json:"consumerId"`
	BindingID      id.ConsumerBindingID   `json:"bindingId"`
	Name           string                 `json:"name"`
	AllowedActions []auth.Action          `json:"allowedActions"`
	ScopeType      auth.ScopeType         `json:"scopeType"`
	ScopeID        string                 `json:"scopeId"`
	ExpiresAt      time.Time              `json:"expiresAt"`
	RotateID       *id.ClientCredentialID `json:"-"`
}

func (s *MachineService) Issue(ctx context.Context, r IssueMachineCredential) (domain.IssuedCredential, error) {
	d, err := s.authorize(ctx, r.WorkspaceID, r.Actor, r.TraceID, auth.ActionBindingManage)
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	now := s.now().UTC()
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 160 || !validCredentialActions(r.AllowedActions) || !r.ExpiresAt.After(now) || r.ExpiresAt.After(now.Add(366*24*time.Hour)) {
		return domain.IssuedCredential{}, domain.ErrInvalidArgument
	}
	switch r.ScopeType {
	case auth.ScopeWorkspace:
		if r.ScopeID != r.WorkspaceID.String() {
			return domain.IssuedCredential{}, domain.ErrInvalidArgument
		}
	case auth.ScopeAsset:
		if _, err := id.ParseAssetID(r.ScopeID); err != nil {
			return domain.IssuedCredential{}, domain.ErrInvalidArgument
		}
	case auth.ScopeRelease:
		if _, err := id.ParseReleaseID(r.ScopeID); err != nil {
			return domain.IssuedCredential{}, domain.ErrInvalidArgument
		}
	default:
		return domain.IssuedCredential{}, domain.ErrInvalidArgument
	}
	p, err := s.repository.LoadPrincipal(ctx, r.WorkspaceID, r.PrincipalID)
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	if p.Kind != auth.PrincipalAgent || p.Status != auth.PrincipalActive {
		return domain.IssuedCredential{}, domain.ErrForbidden
	}
	c, err := s.repository.GetConsumer(ctx, r.WorkspaceID, r.ConsumerID)
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	b, err := s.repository.GetConsumerBinding(ctx, r.WorkspaceID, r.BindingID)
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	if c.Status != dist.ConsumerActive || b.ConsumerID != c.ID || b.Status != dist.BindingActive || b.ExpiresAt != nil && !b.ExpiresAt.After(now) {
		return domain.IssuedCredential{}, domain.ErrForbidden
	}
	credentialID, err := id.NewClientCredentialID()
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	token, err := newMachineToken(credentialID.String())
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	credential := domain.ClientCredential{ID: credentialID, WorkspaceID: r.WorkspaceID, ConsumerID: c.ID, BindingID: b.ID, PrincipalID: p.ID, Name: strings.TrimSpace(r.Name), TokenPrefix: credentialID.String(), VerifierDigest: machineDigest(token), AllowedActions: r.AllowedActions, ScopeType: r.ScopeType, ScopeID: r.ScopeID, IssuedBy: d.PrincipalID, IssuedAt: now, ExpiresAt: r.ExpiresAt.UTC(), RotatedFromID: r.RotateID}
	if err := s.repository.SaveClientCredential(ctx, credential, d.AuthorizationVersion); err != nil {
		return domain.IssuedCredential{}, err
	}
	return domain.IssuedCredential{Credential: credential, Token: token}, nil
}

func validCredentialActions(actions []auth.Action) bool {
	if len(actions) < 1 || len(actions) > 3 {
		return false
	}
	for i, action := range actions {
		if action != auth.ActionSemanticResolve && action != auth.ActionAssetRead && action != auth.ActionSemanticExecute {
			return false
		}
		for _, prior := range actions[:i] {
			if action == prior {
				return false
			}
		}
	}
	return true
}

func (s *MachineService) Rotate(ctx context.Context, workspace id.WorkspaceID, credentialID id.ClientCredentialID, actor, trace string) (domain.IssuedCredential, error) {
	if _, err := s.authorize(ctx, workspace, actor, trace, auth.ActionBindingManage); err != nil {
		return domain.IssuedCredential{}, err
	}
	c, err := s.repository.LoadClientCredential(ctx, credentialID)
	if err != nil {
		return domain.IssuedCredential{}, err
	}
	if c.WorkspaceID != workspace || c.RevokedAt != nil {
		return domain.IssuedCredential{}, domain.ErrNotFound
	}
	return s.Issue(ctx, IssueMachineCredential{WorkspaceID: workspace, Actor: actor, TraceID: trace, PrincipalID: c.PrincipalID, ConsumerID: c.ConsumerID, BindingID: c.BindingID, Name: c.Name, AllowedActions: c.AllowedActions, ScopeType: c.ScopeType, ScopeID: c.ScopeID, ExpiresAt: c.ExpiresAt, RotateID: &c.ID})
}
func (s *MachineService) Revoke(ctx context.Context, workspace id.WorkspaceID, credentialID id.ClientCredentialID, actor, trace string) error {
	d, err := s.authorize(ctx, workspace, actor, trace, auth.ActionBindingManage)
	if err != nil {
		return err
	}
	return s.repository.RevokeClientCredential(ctx, workspace, credentialID, d.PrincipalID, s.now().UTC(), d.AuthorizationVersion)
}

func (s *MachineService) Authenticate(ctx context.Context, token string) (authapp.CredentialLimit, error) {
	fail := func() (authapp.CredentialLimit, error) { return authapp.CredentialLimit{}, domain.ErrUnauthenticated }
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(parts[1]) != 43 {
		return fail()
	}
	credentialID, err := id.ParseClientCredentialID(parts[0])
	if err != nil {
		return fail()
	}
	c, err := s.repository.LoadClientCredential(ctx, credentialID)
	if err != nil {
		return fail()
	}
	now := s.now().UTC()
	if !verifyMachineToken(token, c.VerifierDigest) || c.RevokedAt != nil || !c.ExpiresAt.After(now) {
		return fail()
	}
	p, err := s.repository.LoadPrincipal(ctx, c.WorkspaceID, c.PrincipalID)
	if err != nil || p.Kind != auth.PrincipalAgent || p.Status != auth.PrincipalActive {
		return fail()
	}
	consumer, err := s.repository.GetConsumer(ctx, c.WorkspaceID, c.ConsumerID)
	if err != nil || consumer.Status != dist.ConsumerActive {
		return fail()
	}
	b, err := s.repository.GetConsumerBinding(ctx, c.WorkspaceID, c.BindingID)
	if err != nil || b.ConsumerID != c.ConsumerID || b.Status != dist.BindingActive || b.ExpiresAt != nil && !b.ExpiresAt.After(now) {
		return fail()
	}
	if err := s.repository.TouchClientCredential(ctx, c.ID, now); err != nil {
		return fail()
	}
	return authapp.CredentialLimit{WorkspaceID: c.WorkspaceID, PrincipalID: c.PrincipalID, ConsumerID: c.ConsumerID, BindingID: c.BindingID, CredentialID: c.ID, Actions: c.AllowedActions, ScopeType: c.ScopeType, ScopeID: c.ScopeID}, nil
}
