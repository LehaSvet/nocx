//go:build nocx_local_ssh

package sshsvc_test

// THE ALGORITHM A MODERN SERVER ASKS FOR (nocx-1ebrs.17).
//
// # The defect these are the refusal-of
//
// A vault-held RSA key stopped authenticating on 0.6, where 0.4 connected. The
// key was right, the vault was unlocked, the helper offered it and the
// coordinator signed — with `ssh-rsa`, the SHA-1 algorithm, because the helper's
// reverse signer could only be asked for "a signature", and x/crypto falls back
// to the public key's TYPE for a signer it cannot negotiate with. OpenSSH 8.8
// and later exclude `ssh-rsa` from `PubkeyAcceptedAlgorithms`, so the server's
// journal said:
//
//     userauth_pubkey: signature algorithm ssh-rsa not in PubkeyAcceptedAlgorithms
//
// # Why the fixture is the right instrument
//
// The x/crypto SERVER advertises `PublicKeyAuthAlgorithms` in its
// `server-sig-algs` extension and enforces the same list on the signature it
// receives — the two halves of `PubkeyAcceptedAlgorithms`, one layer down from
// an sshd. A fixture restricted to `rsa-sha2-*` therefore refuses a SHA-1
// signature for the SAME reason the real server did, and answers the question
// without an sshd, a container or a machine of somebody else's.
//
// ed25519 is here beside RSA because its type IS its algorithm: those keys
// authenticated on 0.6 throughout, which is what made a one-machine report of
// "it works here" half right and the defect hard to see.

import (
	"testing"

	"github.com/shady2k/nocx/internal/helper/proto"
	gossh "golang.org/x/crypto/ssh"
)

// rsaSHA2Only is the list a modern sshd accepts for an RSA key, with `ssh-rsa`
// deliberately absent — the configuration the journal line above was measured
// under.
var rsaSHA2Only = []string{gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512}

// TestAProbeWithAnRSAKeySignsWithTheAlgorithmTheServerNegotiated is the
// regression: an RSA key offered to a host that accepts only SHA-2 RSA
// signatures authenticates, and it authenticates because the helper asked the
// coordinator for an algorithm the host listed rather than for the key's type.
//
// RED before the fix, and for the defect's own reason: the signer was not an
// AlgorithmSigner, so x/crypto chose `ssh-rsa`, the fixture refused it, and the
// outcome was `rejected` — "unable to authenticate, attempted methods [none
// publickey], no supported methods remain".
func TestAProbeWithAnRSAKeySignsWithTheAlgorithmTheServerNegotiated(t *testing.T) {
	key := newTestRSAKey(t)
	f := newFixtureAccepting(t, "", key.signer, rsaSHA2Only)
	coord := &coordinator{
		signer: key.signer, verdict: proto.HostKeyTrusted, fingerprint: f.hostKeyFingerprint(),
	}
	stand := newStand(t, coord)

	result, err := stand.probe(t, keyProbeParams(t, f, key.signer))
	if err != nil {
		t.Fatalf("probe with an RSA key against a host that takes only rsa-sha2-*: %v", err)
	}
	if result.Outcome != proto.ProbeAccepted {
		t.Fatalf("outcome = %q (%s), want accepted: an RSA key's TYPE is ssh-rsa (SHA-1), and the "+
			"algorithm the host negotiated is rsa-sha2-256 or -512 (nocx-1ebrs.17)", result.Outcome, result.Detail)
	}

	// THE NEGOTIATION ITSELF, not only its outcome: the coordinator is the
	// party that holds the key, and what it was ASKED for is the value that
	// used to be the key's type.
	asked := coord.signedWith()
	if len(asked) == 0 {
		t.Fatal("the coordinator was never asked to sign: the host accepted something nobody proved")
	}
	for _, algorithm := range asked {
		if algorithm != gossh.KeyAlgoRSASHA256 && algorithm != gossh.KeyAlgoRSASHA512 {
			t.Fatalf("the coordinator was asked to sign with %q, want %q or %q — %q is SHA-1 and is "+
				"exactly what the host refused", algorithm,
				gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512, gossh.KeyAlgoRSA)
		}
	}
	t.Logf("MEASURED an RSA key against a host accepting only %v: the coordinator signed %v",
		rsaSHA2Only, asked)
}

// TestAProbeWithAnED25519KeySignsWithItsOwnAlgorithm is the paired half that
// passed all along, and it is asserted rather than assumed: an ed25519 key's
// type is the algorithm a server negotiates, so the identity of the two is what
// this case proves about the transport, and the RSA case above is what isolates
// the algorithm.
func TestAProbeWithAnED25519KeySignsWithItsOwnAlgorithm(t *testing.T) {
	key := newTestKey(t)
	f := newFixture(t, "", key.signer)
	coord := &coordinator{
		signer: key.signer, verdict: proto.HostKeyTrusted, fingerprint: f.hostKeyFingerprint(),
	}
	stand := newStand(t, coord)

	result, err := stand.probe(t, keyProbeParams(t, f, key.signer))
	if err != nil {
		t.Fatalf("probe with an ed25519 key: %v", err)
	}
	if result.Outcome != proto.ProbeAccepted {
		t.Fatalf("outcome = %q (%s), want accepted: an ed25519 key's type IS its algorithm and this "+
			"path worked before the fix, so a red here means the transport broke, not the algorithm",
			result.Outcome, result.Detail)
	}
	asked := coord.signedWith()
	if len(asked) != 1 || asked[0] != gossh.KeyAlgoED25519 {
		t.Fatalf("the coordinator was asked to sign with %v, want exactly [%s]",
			asked, gossh.KeyAlgoED25519)
	}
}
