package oidc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	adapter "github.com/iiwish/semlia/internal/adapters/oidc"
	"golang.org/x/oauth2"
)

func TestProviderDiscoversIssuerUsesPKCEAndVerifiesIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const clientID = "semlia-test"
	const nonce = "nonce-value"
	const verifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~abc"
	var issuer string
	var receivedVerifier string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			writeTestJSON(response, map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			writeTestJSON(response, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			receivedVerifier = request.Form.Get("code_verifier")
			now := time.Now().UTC()
			payload, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "subject-1", "aud": clientID, "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(), "nonce": nonce, "name": "Alpha User", "email": "alpha@example.com", "email_verified": true})
			signer, signErr := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
			if signErr != nil {
				t.Fatal(signErr)
			}
			signed, signErr := signer.Sign(payload)
			if signErr != nil {
				t.Fatal(signErr)
			}
			compact, signErr := signed.CompactSerialize()
			if signErr != nil {
				t.Fatal(signErr)
			}
			writeTestJSON(response, map[string]any{"access_token": "opaque", "token_type": "Bearer", "expires_in": 3600, "id_token": compact})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	issuer = server.URL

	provider, err := adapter.New(context.Background(), adapter.Config{Issuer: issuer, ClientID: clientID, ClientSecret: "secret", RedirectURL: "https://app.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(provider.AuthorizationURL("state-value", nonce, verifier))
	if err != nil {
		t.Fatal(err)
	}
	query := authorizationURL.Query()
	if query.Get("state") != "state-value" || query.Get("nonce") != nonce || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(verifier) {
		t.Fatalf("authorization query = %v", query)
	}
	claims, err := provider.ExchangeAndVerify(context.Background(), "authorization-code", verifier)
	if err != nil {
		t.Fatal(err)
	}
	if receivedVerifier != verifier {
		t.Fatalf("token endpoint verifier = %q", receivedVerifier)
	}
	if claims.Issuer != issuer || claims.Subject != "subject-1" || claims.Nonce != nonce || !claims.EmailVerified {
		t.Fatalf("verified claims = %+v", claims)
	}
}

func writeTestJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(value)
}
