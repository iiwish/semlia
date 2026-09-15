package discovery

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestCredentialEnvelopeBindsWorkspaceSourceAndVersionWithoutPlaintext(t *testing.T) {
	cipher, err := NewCredentialCipher([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := identity.NewWorkspaceID()
	otherWorkspace, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	password := "alpha-source-password"
	nonce, ciphertext, keyVersion, err := cipher.Seal(workspace, source, 3, domain.CredentialSecret{Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(password)) {
		t.Fatal("credential ciphertext contains plaintext")
	}
	envelope := domain.CredentialEnvelope{WorkspaceID: workspace, SourceConnectionID: source, Version: 3,
		KeyVersion: keyVersion, Algorithm: "AES-256-GCM", Nonce: nonce, Ciphertext: ciphertext}
	opened, err := cipher.Open(envelope)
	if err != nil || opened.Password != password {
		t.Fatalf("open = %#v, %v", opened, err)
	}
	envelope.WorkspaceID = otherWorkspace
	if _, err := cipher.Open(envelope); !errors.Is(err, domain.ErrCredential) {
		t.Fatalf("wrong AAD error = %v", err)
	}
	envelope.WorkspaceID, envelope.KeyVersion = workspace, 99
	if _, err := cipher.Open(envelope); !errors.Is(err, domain.ErrCredential) {
		t.Fatalf("wrong key version error = %v", err)
	}
}

func TestCredentialCipherRequiresStrongRootKey(t *testing.T) {
	if _, err := NewCredentialCipher([]byte("short")); err == nil {
		t.Fatal("short root key accepted")
	}
}
