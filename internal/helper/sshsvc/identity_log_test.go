//go:build nocx_local_ssh

package sshsvc_test

// THE LINE THAT MAKES THE NEXT OCCURRENCE ONE READ (nocx-1ebrs.18).
//
// The defect it exists for was a night of bisection against a log that said a
// public key had been attempted and not WHICH one: "the wrong key was offered"
// and "the right key was refused" are two different defects that line cannot
// tell apart, and the far host's journal says only that it said no. The
// helper names every identity it offers, by FINGERPRINT, and never by material
// — a fingerprint is a one-way function of a public key, which the server sees
// anyway.

import (
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/helper/proto"
	"github.com/shady2k/nocx/internal/log/logtest"
	gossh "golang.org/x/crypto/ssh"
)

// TestTheHelperNamesEveryIdentityItOffersAndNoKeyMaterial is nocx-1ebrs.18's
// third part: the log says WHICH identity was offered, by fingerprint, and says
// nothing that is secret.
//
// It is asserted on the log a person would read after the event, because the
// defect it exists for was a night of bisection against a line that said only
// "a public key was attempted": "the wrong key was offered" and "the right key
// was refused" are the two answers that line cannot tell apart.
func TestTheHelperNamesEveryIdentityItOffersAndNoKeyMaterial(t *testing.T) {
	key := newTestRSAKey(t)
	f := newFixtureAccepting(t, "", key.signer, rsaSHA2Only)
	logger := logtest.Slog(t)
	coord := &coordinator{
		signer: key.signer, verdict: proto.HostKeyTrusted, fingerprint: f.hostKeyFingerprint(),
	}
	stand := newStandLogging(t, coord, logger)

	if _, err := stand.probe(t, keyProbeParams(t, f, key.signer)); err != nil {
		t.Fatalf("probe with an RSA key: %v", err)
	}

	want := gossh.FingerprintSHA256(key.signer.PublicKey())
	text := logtest.TextSlog(logger)
	if !strings.Contains(text, want) {
		t.Fatalf("the helper's log does not name the identity it offered (%s); it said:\n%s", want, text)
	}
	// NAMED AS OFFERED, and again as SIGNED: the second line carries the
	// algorithm the handshake negotiated, which is the value that would have
	// printed `ssh-rsa` and ended the investigation on the first read.
	if !strings.Contains(text, gossh.KeyAlgoRSASHA256) && !strings.Contains(text, gossh.KeyAlgoRSASHA512) {
		t.Fatalf("the helper's log does not say which signature algorithm it used; it said:\n%s", text)
	}
	// AND NOTHING THAT IS SECRET. The private half's PEM never reaches the log,
	// by line or by any of its own lines: the assertion is on the PEM text and
	// on the body without its armour, because either would be the key.
	body := strings.Split(string(key.pem), "\n")
	for _, line := range body {
		if len(strings.TrimSpace(line)) < 12 || strings.HasPrefix(line, "-----") {
			continue
		}
		if strings.Contains(text, strings.TrimSpace(line)) {
			t.Fatalf("the helper's log carries private key material; it said:\n%s", text)
		}
	}
}
