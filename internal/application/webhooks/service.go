package webhooks

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/application/jobs"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/webhooks"
	"github.com/iiwish/semlia/pkg/identity"
	"io"
	"strconv"
	"strings"
	"time"
)

type Repository interface {
	SaveWebhookSubscription(context.Context, domain.Subscription, int, int64, string) error
	ListWebhookSubscriptions(context.Context, identity.WorkspaceID) ([]domain.Subscription, error)
	GetWebhookSubscription(context.Context, identity.WorkspaceID, identity.ID) (domain.Subscription, error)
	ListWebhookDeliveries(context.Context, identity.WorkspaceID) ([]domain.Delivery, error)
	EnqueueWebhookDeliveries(context.Context, jobs.OutboxEvent, []byte, string) error
	ClaimWebhookDelivery(context.Context, string, time.Time) (*domain.Delivery, error)
	FinishWebhookDelivery(context.Context, domain.Delivery, string, time.Time, int, string) error
	ReplayWebhookDelivery(context.Context, identity.WorkspaceID, identity.ID, int, int64, string, time.Time) (domain.Delivery, error)
}
type Transport interface {
	Validate(context.Context, string) error
	Send(context.Context, string, []byte, map[string]string) (int, error)
}
type Service struct {
	repo      Repository
	auth      authapp.Evaluator
	transport Transport
	aead      cipher.AEAD
	now       func() time.Time
}

func NewService(repo Repository, authorizer authapp.Evaluator, transport Transport, key []byte) (*Service, error) {
	if len(key) < 32 {
		return nil, domain.ErrInvalidArgument
	}
	derived, err := hkdf.Key(sha256.New, key, nil, "semlia.webhook-signing.v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Service{repo, authorizer, transport, aead, time.Now}, nil
}
func (s *Service) authorize(ctx context.Context, w identity.WorkspaceID, actor, trace string, action auth.Action) (auth.Decision, error) {
	if s.auth == nil {
		return auth.Decision{}, domain.ErrInvalidArgument
	}
	d, err := s.auth.Evaluate(ctx, authapp.EvaluationRequest{WorkspaceID: w, PrincipalRef: actor, TraceID: trace, Action: action, Resource: auth.Resource{Type: auth.ScopeWorkspace, ID: w.UUID()}})
	if err != nil {
		return d, err
	}
	if !d.Allowed {
		return d, &auth.DenialError{Decision: d}
	}
	return d, nil
}

type SaveRequest struct {
	WorkspaceID     identity.WorkspaceID
	Actor, TraceID  string
	ID              identity.ID
	Name, Endpoint  string
	Enabled         bool
	EventTypes      []string
	ExpectedVersion int
	Rotate          bool
}

func (s *Service) Save(ctx context.Context, r SaveRequest) (domain.IssuedSubscription, error) {
	d, err := s.authorize(ctx, r.WorkspaceID, r.Actor, r.TraceID, auth.ActionBindingManage)
	if err != nil {
		return domain.IssuedSubscription{}, err
	}
	var sub domain.Subscription
	now := s.now().UTC()
	if r.ID.IsZero() {
		id, err := identity.New(identity.WebhookSubscription)
		if err != nil {
			return domain.IssuedSubscription{}, err
		}
		sub = domain.Subscription{ID: id, WorkspaceID: r.WorkspaceID, CreatedBy: d.PrincipalID, CreatedAt: now, SigningVersion: 1}
	} else {
		sub, err = s.repo.GetWebhookSubscription(ctx, r.WorkspaceID, r.ID)
		if err != nil {
			return domain.IssuedSubscription{}, err
		}
		if sub.Version != r.ExpectedVersion {
			return domain.IssuedSubscription{}, domain.ErrConflict
		}
	}
	if r.Rotate {
		sub.SigningVersion++
	} else {
		if strings.TrimSpace(r.Name) == "" || len(r.Name) > 160 || len(r.EventTypes) == 0 || len(r.EventTypes) > 2 {
			return domain.IssuedSubscription{}, domain.ErrInvalidArgument
		}
		for _, kind := range r.EventTypes {
			if kind != "release.published" && kind != "catalog.asset.changed" {
				return domain.IssuedSubscription{}, domain.ErrInvalidArgument
			}
		}
		if err := s.transport.Validate(ctx, r.Endpoint); err != nil {
			return domain.IssuedSubscription{}, domain.ErrInvalidArgument
		}
		sub.Name, sub.Endpoint, sub.Enabled, sub.EventTypes = strings.TrimSpace(r.Name), r.Endpoint, r.Enabled, append([]string(nil), r.EventTypes...)
	}
	secret := ""
	if r.ID.IsZero() || r.Rotate {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return domain.IssuedSubscription{}, err
		}
		secret = base64.RawURLEncoding.EncodeToString(bytes)
		sub.SecretSuffix = secret[len(secret)-4:]
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return domain.IssuedSubscription{}, err
		}
		sub.SecretEnvelope = s.aead.Seal(nonce, nonce, []byte(secret), secretAAD(sub.WorkspaceID, sub.ID, sub.SigningVersion))
	}
	sub.Version++
	sub.UpdatedAt = now
	if err := s.repo.SaveWebhookSubscription(ctx, sub, r.ExpectedVersion, d.AuthorizationVersion, d.PrincipalID.String()); err != nil {
		return domain.IssuedSubscription{}, err
	}
	return domain.IssuedSubscription{Subscription: sub, SigningSecret: secret}, nil
}
func (s *Service) List(ctx context.Context, w identity.WorkspaceID, actor, trace string) ([]domain.Subscription, error) {
	if _, err := s.authorize(ctx, w, actor, trace, auth.ActionBindingRead); err != nil {
		return nil, err
	}
	return s.repo.ListWebhookSubscriptions(ctx, w)
}
func (s *Service) Deliveries(ctx context.Context, w identity.WorkspaceID, actor, trace string) ([]domain.Delivery, error) {
	if _, err := s.authorize(ctx, w, actor, trace, auth.ActionRuntimeRead); err != nil {
		return nil, err
	}
	return s.repo.ListWebhookDeliveries(ctx, w)
}
func (s *Service) Replay(ctx context.Context, w identity.WorkspaceID, id identity.ID, attempt int, actor, trace string) (domain.Delivery, error) {
	decision, err := s.authorize(ctx, w, actor, trace, auth.ActionRuntimeManage)
	if err != nil {
		return domain.Delivery{}, err
	}
	if id.Prefix() != identity.WebhookDelivery || attempt < 1 {
		return domain.Delivery{}, domain.ErrInvalidArgument
	}
	return s.repo.ReplayWebhookDelivery(ctx, w, id, attempt, decision.AuthorizationVersion, decision.PrincipalID.String(), s.now().UTC())
}
func (s *Service) Publish(ctx context.Context, event jobs.OutboxEvent) error {
	data := map[string]string{}
	var envelope struct {
		SpecVersion string                     `json:"specVersion"`
		ID          string                     `json:"id"`
		Type        string                     `json:"type"`
		Source      string                     `json:"source"`
		WorkspaceID string                     `json:"workspaceId"`
		Time        time.Time                  `json:"time"`
		TraceID     string                     `json:"traceId"`
		Data        map[string]json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || decoder.Decode(&struct{}{}) != io.EOF || envelope.SpecVersion != "semlia.events/v1" || envelope.ID != event.ID.String() || envelope.Type != event.Type || envelope.Source != "urn:semlia:control-plane" || envelope.WorkspaceID != event.WorkspaceID.String() || envelope.TraceID != event.TraceID || envelope.Time.IsZero() {
		return domain.ErrInvalidArgument
	}
	for key, prefix := range map[string]identity.Prefix{"assetId": identity.Asset, "releaseId": identity.Release} {
		var value string
		if json.Unmarshal(envelope.Data[key], &value) == nil {
			if _, err := identity.Parse(prefix, value); err == nil {
				data[key] = value
			}
		}
	}
	safe, _ := json.Marshal(data)
	body, err := json.Marshal(domain.Envelope{Version: "1.0.0", ID: event.ID, Type: event.Type, WorkspaceID: event.WorkspaceID, OccurredAt: event.CreatedAt, TraceID: event.TraceID, Data: safe})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	return s.repo.EnqueueWebhookDeliveries(ctx, event, body, "sha256:"+hex.EncodeToString(digest[:]))
}
func secretAAD(w identity.WorkspaceID, id identity.ID, version int) []byte {
	return []byte(w.String() + "/" + id.String() + "/" + strconv.Itoa(version))
}
func Signature(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) RunOne(ctx context.Context, owner string) (bool, error) {
	if owner == "" {
		return false, domain.ErrInvalidArgument
	}
	delivery, err := s.repo.ClaimWebhookDelivery(ctx, owner, s.now().UTC())
	if err != nil || delivery == nil {
		return false, err
	}
	code := ""
	status := 0
	nonceSize := s.aead.NonceSize()
	if delivery.Attempt > delivery.MaxAttempts {
		code = "ATTEMPTS_EXHAUSTED"
	} else if delivery.ErrorCode == "SUBSCRIPTION_DISABLED" || delivery.ErrorCode == "SUBSCRIPTION_CHANGED" {
		code = delivery.ErrorCode
	} else if len(delivery.SecretEnvelope) < nonceSize {
		code = "SIGNING_UNAVAILABLE"
	} else {
		secret, err := s.aead.Open(nil, delivery.SecretEnvelope[:nonceSize], delivery.SecretEnvelope[nonceSize:], secretAAD(delivery.WorkspaceID, delivery.SubscriptionID, delivery.SigningVersion))
		if err != nil {
			code = "SIGNING_UNAVAILABLE"
		} else {
			timestamp := strconv.FormatInt(s.now().Unix(), 10)
			status, err = s.transport.Send(ctx, delivery.Endpoint, delivery.Envelope, map[string]string{"Idempotency-Key": delivery.EventID.String(), "X-Semlia-Event-ID": delivery.EventID.String(), "X-Semlia-Timestamp": timestamp, "X-Semlia-Signature": Signature(string(secret), timestamp, delivery.Envelope), "X-Semlia-Signing-Version": strconv.Itoa(delivery.SigningVersion)})
			if err != nil {
				code = "DELIVERY_TRANSPORT_FAILED"
			} else if status < 200 || status >= 300 {
				code = "DELIVERY_HTTP_FAILED"
			}
		}
	}
	err = s.repo.FinishWebhookDelivery(ctx, *delivery, owner, s.now().UTC(), status, code)
	if errors.Is(err, domain.ErrConflict) {
		return true, nil
	}
	return true, err
}
func (s *Service) Run(ctx context.Context, owner string) error {
	for {
		processed, err := s.RunOne(ctx, owner)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return fmt.Errorf("webhook worker failed: %w", err)
		}
		if processed {
			continue
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
