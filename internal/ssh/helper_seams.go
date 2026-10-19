package ssh

// Two seams this package owes a caller that has no *gossh.Client to hand it:
// answering what a host key IS, and signing a challenge with a stored key.
//
// Both exist for the same reason and neither is a new mechanism. A helper that
// dials the connection cannot consult this package's dial path — it has no
// known_hosts file to read (`ssh/knownhosts` is a forbidden import in the
// deployed artifact, internal/helper/deploy/dependency_test.go) and it must
// never hold key material at rest — so the two questions its handshake needs
// answered are answered here, by the code that already answers them, and cross
// to the helper as ordinary op results (proto/ssh_service.go).
//
// # What is deliberately NOT here
//
// There is no seam that hands out a gossh.HostKeyCallback, and no seam that
// hands out a *gossh.ClientConfig. Both would move the DECISION into the
// helper: a callback decides "accept or refuse", and the coordinator's answer
// to a host key it has never seen is a question for a person, not a predicate
// the helper can run. What crosses instead is a verdict — trusted, changed,
// unknown (proto.HostKeyVerdict) — and the helper acts on the answer rather
// than on a rule.

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"

	gossh "golang.org/x/crypto/ssh"
)

// addr is a net.Addr for a caller that only ever had the address STRING.
//
// knownhosts needs one — its check splits the peer address to build the
// second, IP-keyed lookup it tries after the hostname one — so a check given
// nil panics rather than refusing, which is how this type came to exist. The
// string IS the address: for a direct route the dial address and the peer
// address spell the same host and port, and a caller that resolved a route
// passes exactly what it will dial (and look up) under.
type addrString string

func (a addrString) Network() string { return "tcp" }
func (a addrString) String() string  { return string(a) }

// CheckHostKey answers how the offered key stands against this client's
// known_hosts, in the vocabulary this package already uses for it:
//
//	nil                  the key is the one recorded for addr (trusted)
//	*ErrUnknownHostKey   nothing is recorded for addr
//	*ErrHostKeyMismatch  a key IS recorded, and this is a different one
//
// The two error types carry the evidence a person is shown — the offered
// fingerprint, the recorded one, and the offered key's wire bytes so the
// accept path can write without re-probing — and they are the SAME values the
// dial path produces, because this calls the same callback the dial path
// installs (hostKeyCallbackFor). A second derivation of "is this the key we
// recorded" would be the defect AD-8 names, and it would be the one place in
// this program where the answer could be wrong in the quiet direction.
//
// addr is the STORAGE identity, not the dial address: a route through a jump
// host records its key under an opaque digest (knownHostsTargetAddr), so the
// caller that knows the route passes the identity the next connection will
// look up. A direct route passes its own address, which is what the
// coordinator does today.
//
// A malformed key blob is an error and not a verdict: it means the peer sent
// something that is not a public key, which is a fact about this handshake and
// not about the trust store.
func (rc *RealClient) CheckHostKey(addr string, keyBlob []byte) error {
	key, err := gossh.ParsePublicKey(keyBlob)
	if err != nil {
		return fmt.Errorf("host key: %w", err)
	}
	cb, err := rc.hostKeyCallbackFor(knownHostsTargetAddr(addr, nil))
	if err != nil {
		return fmt.Errorf("host key callback: %w", err)
	}
	// The peer address is the same string, and it has to be a real net.Addr:
	// see addrString for what a nil one costs.
	return cb(addr, addrString(addr), key)
}

// DialAuth dials a resolved destination ONCE, through its route when it has
// one, and answers the authenticated client. It is declared in ssh_pooled.go,
// beside the pool it deliberately does not use: a probe dials, authenticates
// and closes, and the connection it left behind would be one whose lifetime
// nothing owns.

// ErrSignatureAlgorithm is the answer when a key cannot produce the signature
// algorithm the peer negotiated: the key is not the problem, the request is.
//
// It is a typed error rather than a message because the party that acts on it
// is a wire handler one package away, and it answers DIFFERENTLY for this than
// for "the vault would not open": a bad request is the helper's bug, a locked
// key is a person's prompt (nocx-1ebrs.17).
var ErrSignatureAlgorithm = errors.New("ssh: the key cannot produce the negotiated signature algorithm")

// SignWithStoredKey signs challenge with the private key a config's
// KeySecretID names, answering the signature in the wire encoding of an ssh
// signature (string format, string blob) — the form x/crypto/ssh's own
// Unmarshal reads back into a Signature, so the party that asked can hand it
// to the library unchanged.
//
// cfg carries the material references and nothing else that matters here:
// KeySecretID, PassphraseSecretID when the key is encrypted, and Secrets, the
// stanced resolver. The signer is built by the same function the dial path
// builds it with (storedKeySigner), so the passphrase stays inside the vault's
// Use callback, the sealed-vault error propagates as itself, and an encrypted
// key with no passphrase is *ErrEncryptedKey rather than a parse failure.
//
// ctx is the caller's, and it is not decoration: the read is STANCED, so a
// sealed vault raises the unlock a person answers through, and that wait is
// bounded by whatever the caller is willing to wait for.
//
// The key's bytes never leave this call: what returns is a signature, which is
// what a signature is for. The ALGORITHM comes back beside it because the
// caller asked for one and the key is the only party that can settle whether
// they agree — a signature made with a key other than the one named is a
// signature for a handshake that never happened, and a signature made with an
// algorithm the peer did not negotiate is one it refuses (signWithAlgorithm's
// comment carries the measurement).
//
// `algorithm` is the signature algorithm the PEER negotiated — `rsa-sha2-512`,
// `rsa-sha2-256`, `ssh-ed25519` — and not the key's type: for an RSA key those
// are three different names for one key, and the type is the one modern
// servers no longer accept.
func (rc *RealClient) SignWithStoredKey(ctx context.Context, cfg *ConnectConfig, challenge []byte, algorithm string) (signature []byte, signed string, err error) {
	signer, err := rc.storedKeySigner(ctx, cfg)
	if err != nil {
		return nil, "", err
	}
	sig, signed, err := signWithAlgorithm(signer, challenge, algorithm)
	if err != nil {
		return nil, "", fmt.Errorf("sign with the stored key: %w", err)
	}
	return gossh.Marshal(sig), signed, nil
}

// signWithAlgorithm makes ONE signature with the algorithm the peer
// NEGOTIATED, and answers the algorithm the signer actually used.
//
// # Why the algorithm cannot be the key's type any more (nocx-1ebrs.17)
//
// For an RSA key the type is `ssh-rsa`, which is SHA-1, and OpenSSH has
// excluded it from `PubkeyAcceptedAlgorithms` since 8.8. What a modern server
// accepts for an RSA key is `rsa-sha2-256` or `rsa-sha2-512`, and the choice is
// the HANDSHAKE's: x/crypto picks it from the server's own `server-sig-algs`
// extension and hands it to the signer. A signer that can only be asked for
// "a signature" therefore sends SHA-1 by accident, and the server's journal
// says `signature algorithm ssh-rsa not in PubkeyAcceptedAlgorithms` while the
// credential itself was always correct. The helper's reverse signer is one
// process away from the key and is asked by NAME for exactly this reason.
//
// The ANSWER is `sig.Format`, the algorithm the signer wrote into the
// signature, rather than the one that was requested: the caller compares the
// two, and a signer that quietly substituted another algorithm is a fact the
// caller must see rather than a fact this function asserts about itself.
//
// An empty algorithm asks for the signer's own default, which is `Signer.Sign`
// and the key's type. That is the shape a caller uses when it has nothing
// negotiated to pass, and it is the fallback the interface documents.
func signWithAlgorithm(signer gossh.Signer, challenge []byte, algorithm string) (*gossh.Signature, string, error) {
	if algorithm == "" {
		sig, err := signer.Sign(rand.Reader, challenge)
		if err != nil {
			return nil, "", err
		}
		return sig, sig.Format, nil
	}
	as, ok := signer.(gossh.AlgorithmSigner)
	if !ok {
		// A signer that cannot name an algorithm cannot make this signature
		// honestly, and signing with its own default instead would produce a
		// signature the peer rejects with a message about algorithms rather
		// than about the party that could not provide one.
		return nil, "", fmt.Errorf("%w: this key signs as %s and cannot be asked for %s",
			ErrSignatureAlgorithm, signer.PublicKey().Type(), algorithm)
	}
	if multi, ok := signer.(gossh.MultiAlgorithmSigner); ok && !slices.Contains(multi.Algorithms(), algorithm) {
		// Asked before signing rather than after failing, because this is the
		// one arm that can say WHY: the key is fine, the request named an
		// algorithm this key format does not have.
		return nil, "", fmt.Errorf("%w: a %s key supports %v and was asked for %s",
			ErrSignatureAlgorithm, signer.PublicKey().Type(), multi.Algorithms(), algorithm)
	}
	sig, err := as.SignWithAlgorithm(rand.Reader, challenge, algorithm)
	if err != nil {
		return nil, "", err
	}
	return sig, sig.Format, nil
}
