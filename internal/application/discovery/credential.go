package discovery

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

const credentialKeyVersion = 1

type CredentialCipher struct {
	aead       cipher.AEAD
	keyVersion int
}

func NewCredentialCipher(rootSecret []byte) (*CredentialCipher, error) {
	if len(rootSecret) < 32 {
		return nil, errors.New("credential encryption key must be at least 32 bytes")
	}
	key, err := hkdf.Key(sha256.New, rootSecret, nil, "semlia.source-credential.v1", 32)
	if err != nil {
		return nil, fmt.Errorf("derive credential key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize credential envelope: %w", err)
	}
	return &CredentialCipher{aead: aead, keyVersion: credentialKeyVersion}, nil
}

func (cipher *CredentialCipher) Seal(
	workspace identity.WorkspaceID,
	source identity.SourceConnectionID,
	version int64,
	secret domain.CredentialSecret,
) (nonce, ciphertext []byte, keyVersion int, err error) {
	if cipher == nil || version < 1 || secret.Password == "" {
		return nil, nil, 0, domain.ErrCredential
	}
	plaintext, err := json.Marshal(secret)
	if err != nil {
		return nil, nil, 0, domain.ErrCredential
	}
	nonce = make([]byte, cipher.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, 0, fmt.Errorf("generate credential nonce: %w", err)
	}
	ciphertext = cipher.aead.Seal(nil, nonce, plaintext, credentialAAD(workspace, source, version, cipher.keyVersion))
	return nonce, ciphertext, cipher.keyVersion, nil
}

func (cipher *CredentialCipher) Open(envelope domain.CredentialEnvelope) (domain.CredentialSecret, error) {
	if cipher == nil || envelope.KeyVersion != cipher.keyVersion || envelope.Algorithm != "AES-256-GCM" ||
		len(envelope.Nonce) != cipher.aead.NonceSize() || len(envelope.Ciphertext) == 0 {
		return domain.CredentialSecret{}, domain.ErrCredential
	}
	plaintext, err := cipher.aead.Open(nil, envelope.Nonce, envelope.Ciphertext, credentialAAD(
		envelope.WorkspaceID, envelope.SourceConnectionID, envelope.Version, envelope.KeyVersion,
	))
	if err != nil {
		return domain.CredentialSecret{}, domain.ErrCredential
	}
	var secret domain.CredentialSecret
	if json.Unmarshal(plaintext, &secret) != nil || secret.Password == "" {
		return domain.CredentialSecret{}, domain.ErrCredential
	}
	return secret, nil
}

func credentialAAD(workspace identity.WorkspaceID, source identity.SourceConnectionID, version int64, keyVersion int) []byte {
	result := []byte("semlia.source-credential-envelope.v1\x00" + workspace.String() + "\x00" + source.String())
	buffer := make([]byte, 12)
	binary.BigEndian.PutUint64(buffer[:8], uint64(version))
	binary.BigEndian.PutUint32(buffer[8:], uint32(keyVersion))
	return append(result, buffer...)
}
