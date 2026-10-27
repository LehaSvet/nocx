package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/shady2k/nocx/internal/coordinator"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/update"
	"github.com/shady2k/nocx/internal/version"
)

// windowUpdater is the single construction used by startup AND the packaged
// executable's release diagnostic. No test-only HTTP client or trust root is
// substituted by the diagnostic; a nil production client must fail the gate.
type windowUpdater struct {
	updater  update.Updater
	fetch    *update.GitHubManifestFetcher
	probe    *coordinator.LaunchProbe
	evidence *recordedManifestFetcher
}

func newWindowUpdater(execPath string, logger log.Logger) (windowUpdater, error) {
	keyring, err := update.ReleaseKeyring()
	// Startup must reconcile before the coordinator launcher runs. The pair
	// probe therefore starts empty and is attached to the launch result later;
	// until then it refuses to certify update health (D4).
	probe := coordinator.NewLaunchProbe()
	fetch := update.NewGitHubManifestFetcher(nil)
	evidence := &recordedManifestFetcher{source: fetch}
	updater := update.NewUpdater(update.UpdaterConfig{
		Platform: update.NewPlatform(), Fetcher: evidence, Keyring: keyring,
		CurrentVersion: version.Version, InstallPath: upgradeInstallPath(execPath),
		Coordinator: probe, Logger: logger,
	})
	return windowUpdater{updater: updater, fetch: fetch, probe: probe, evidence: evidence}, err
}

// The wrapper retains precisely the bytes Check fetched. This is not a second
// request that could succeed while the updater's own fetch is broken. Startup
// doesn't read these snapshots; only the one-shot diagnostic does.
type recordedManifestFetcher struct {
	source update.ManifestFetcher
	mu     sync.Mutex
	body   []byte
	sig    []byte
}

func (f *recordedManifestFetcher) Fetch(ctx context.Context) ([]byte, []byte, error) {
	body, sig, err := f.source.Fetch(ctx)
	f.mu.Lock()
	f.body, f.sig = body, sig
	f.mu.Unlock()
	return body, sig, err
}

// releaseDiagnostic runs before Wails, GTK, a coordinator, a profile, or an OS
// keychain exists. It never reconciles or applies an update to the runner.
func releaseDiagnostic(ctx context.Context, args []string, out io.Writer) (bool, error) {
	if len(args) != 1 {
		return false, nil
	}
	switch args[0] {
	case "--build-info":
		return true, json.NewEncoder(out).Encode(version.Info())
	case "--verify-release-manifest":
		if version.Info().Development {
			return true, errors.New("updater: unstamped development build cannot prove manifest verification")
		}
		execPath, err := os.Executable()
		if err != nil {
			return true, err
		}
		wiring, err := newWindowUpdater(execPath, log.NewSlogAdapter(nil))
		if err != nil {
			return true, err
		}
		checkCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if _, checkErr := wiring.updater.Check(checkCtx); checkErr != nil {
			return true, checkErr
		}
		wiring.evidence.mu.Lock()
		body, sig := wiring.evidence.body, wiring.evidence.sig
		wiring.evidence.mu.Unlock()
		if len(body) == 0 || len(sig) == 0 {
			return true, errors.New("updater: Check returned without fetching a signed manifest")
		}
		keys, err := update.ReleaseKeyring()
		if err != nil {
			return true, err
		}
		manifest, err := update.VerifyManifest(body, string(sig), keys)
		if err != nil {
			return true, fmt.Errorf("updater: %w", err)
		}
		sum := sha256.Sum256(body)
		return true, json.NewEncoder(out).Encode(struct {
			Manifest  *update.Manifest `json:"manifest"`
			SHA256    string           `json:"sha256"`
			Signature string           `json:"signature"`
			Source    string           `json:"source"`
		}{manifest, hex.EncodeToString(sum[:]), string(sig), wiring.fetch.BaseURL})
	default:
		return false, nil
	}
}
