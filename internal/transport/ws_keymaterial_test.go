package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// testPrivateKeyPEM generates a new ed25519 key and returns its PEM-encoded
// private key text and its SHA256 fingerprint.
func testPrivateKeyPEM(t *testing.T) (pemOut, fingerprint string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	fingerprint = gossh.FingerprintSHA256(signer.PublicKey())
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return string(pem.EncodeToMemory(block)), fingerprint
}

// encryptedKeyPEMFixture caches immutable encrypted test material by
// passphrase. `go test -count=N` reruns tests in one process, and generating
// this fixture invokes bcrypt_pbkdf; the source key is not part of the test.
type encryptedKeyPEMFixture struct {
	once        sync.Once
	pem         string
	fingerprint string
	err         error
}

var encryptedKeyPEMFixtures = struct {
	sync.Mutex
	byPassphrase map[string]*encryptedKeyPEMFixture
}{byPassphrase: make(map[string]*encryptedKeyPEMFixture)}

// testEncryptedKeyPEM returns a stable fixture for a passphrase so repeated
// test runs do not redo expensive OpenSSH encryption.
func testEncryptedKeyPEM(t *testing.T, passphrase string) (pemOut, fingerprint string) {
	t.Helper()
	encryptedKeyPEMFixtures.Lock()
	fixture := encryptedKeyPEMFixtures.byPassphrase[passphrase]
	if fixture == nil {
		fixture = &encryptedKeyPEMFixture{}
		encryptedKeyPEMFixtures.byPassphrase[passphrase] = fixture
	}
	encryptedKeyPEMFixtures.Unlock()
	fixture.once.Do(func() {
		fixture.pem, fixture.fingerprint, fixture.err = generateEncryptedKeyPEM(passphrase)
	})
	if fixture.err != nil {
		t.Fatalf("generate encrypted key fixture: %v", fixture.err)
	}
	return fixture.pem, fixture.fingerprint
}

func generateEncryptedKeyPEM(passphrase string) (pemOut, fingerprint string, err error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return "", "", fmt.Errorf("create signer: %w", err)
	}
	fingerprint = gossh.FingerprintSHA256(signer.PublicKey())

	// Marshal with passphrase uses OpenSSH format which stores the
	// public key unencrypted, so ParseRawPrivateKey can extract it.
	block, err := gossh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	if err != nil {
		return "", "", fmt.Errorf("marshal encrypted key: %w", err)
	}
	return string(pem.EncodeToMemory(block)), fingerprint, nil
}

func TestEncryptedKeyPEM_CachesFixturePerPassphrase(t *testing.T) {
	firstPEM, firstFingerprint := testEncryptedKeyPEM(t, "fixture-cache-test")
	secondPEM, secondFingerprint := testEncryptedKeyPEM(t, "fixture-cache-test")
	if secondPEM != firstPEM || secondFingerprint != firstFingerprint {
		t.Fatal("encrypted key fixture was regenerated for the same passphrase")
	}
}

// --- parsePrivateKeyMaterial tests ---

func TestParsePrivateKeyMaterial_ValidUnencrypted(t *testing.T) {
	pem, expectedFP := testPrivateKeyPEM(t)
	fp, wantsPassphrase, err := parsePrivateKeyMaterial(pem)
	if err != nil {
		t.Fatalf("parsePrivateKeyMaterial(unencrypted): %v", err)
	}
	if fp != expectedFP {
		t.Fatalf("fingerprint mismatch: got %q, want %q", fp, expectedFP)
	}
	if wantsPassphrase {
		t.Fatal("passphraseWanted = true for an unencrypted key")
	}
}

func TestParsePrivateKeyMaterial_ValidEncrypted(t *testing.T) {
	pem, expectedFP := testEncryptedKeyPEM(t, "test-passphrase")
	fp, wantsPassphrase, err := parsePrivateKeyMaterial(pem)
	if err != nil {
		t.Fatalf("parsePrivateKeyMaterial(encrypted): %v", err)
	}
	if fp != expectedFP {
		t.Fatalf("fingerprint mismatch: got %q, want %q", fp, expectedFP)
	}
	if !wantsPassphrase {
		t.Fatal("passphraseWanted = false for an encrypted key")
	}
}

func TestParsePrivateKeyMaterial_InvalidText(t *testing.T) {
	_, _, err := parsePrivateKeyMaterial("this is not a private key")
	if err == nil {
		t.Fatal("parsePrivateKeyMaterial should reject non-key text")
	}
	var invalidKey *errInvalidKeyMaterial
	if !errors.As(err, &invalidKey) {
		t.Fatal("error should be *errInvalidKeyMaterial")
	}
}

// Test that arbitrary binary data is rejected.
func TestParsePrivateKeyMaterial_BinaryData(t *testing.T) {
	_, _, err := parsePrivateKeyMaterial("\x00\x01\x02\x03")
	if err == nil {
		t.Fatal("parsePrivateKeyMaterial should reject binary data")
	}
}

// --- saveKeyMaterial RPC tests ---
