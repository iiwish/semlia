// Package oidc adapts a standards-compliant OpenID Connect provider to Semlia's
// identity application port.
package oidc

import (
	"context"
	"errors"
	"strings"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	"golang.org/x/oauth2"
)

type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type Provider struct {
	issuer   string
	oauth2   oauth2.Config
	verifier *coreoidc.IDTokenVerifier
}

func New(ctx context.Context, config Config) (*Provider, error) {
	issuer, err := domain.NormalizeIssuer(config.Issuer)
	if err != nil || strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.RedirectURL) == "" {
		return nil, errors.New("valid OIDC issuer, client ID and redirect URL are required")
	}
	discovered, err := coreoidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, errors.New("OIDC provider discovery failed")
	}
	return &Provider{
		issuer: issuer,
		oauth2: oauth2.Config{
			ClientID: strings.TrimSpace(config.ClientID), ClientSecret: config.ClientSecret,
			Endpoint: discovered.Endpoint(), RedirectURL: strings.TrimSpace(config.RedirectURL),
			Scopes: []string{coreoidc.ScopeOpenID, "profile", "email"},
		},
		verifier: discovered.Verifier(&coreoidc.Config{ClientID: strings.TrimSpace(config.ClientID)}),
	}, nil
}

func (provider *Provider) Issuer() string { return provider.issuer }

func (provider *Provider) AuthorizationURL(state, nonce, verifier string) string {
	return provider.oauth2.AuthCodeURL(
		state,
		coreoidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
}

func (provider *Provider) ExchangeAndVerify(ctx context.Context, code, pkceVerifier string) (domain.OIDCClaims, error) {
	token, err := provider.oauth2.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		return domain.OIDCClaims{}, errors.New("OIDC authorization code exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return domain.OIDCClaims{}, errors.New("OIDC response did not include an ID token")
	}
	idToken, err := provider.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return domain.OIDCClaims{}, errors.New("OIDC ID token verification failed")
	}
	var claims struct {
		Issuer        string `json:"iss"`
		Subject       string `json:"sub"`
		Name          string `json:"name"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Nonce         string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return domain.OIDCClaims{}, errors.New("OIDC ID token claims are invalid")
	}
	return domain.OIDCClaims{
		Issuer: claims.Issuer, Subject: claims.Subject, DisplayName: claims.Name,
		Email: claims.Email, EmailVerified: claims.EmailVerified, Nonce: claims.Nonce,
	}, nil
}
