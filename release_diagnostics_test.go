package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/coordinator"
	"github.com/shady2k/nocx/internal/log/logtest"
	"github.com/shady2k/nocx/internal/version"
)

func TestReleaseDiagnosticsBuildInfo(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "diagnostic")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	handled, err := releaseDiagnostic(context.Background(), []string{"--build-info"}, f)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var info version.BuildInfo
	if err := json.NewDecoder(f).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Version != version.Version {
		t.Fatalf("Version=%q", info.Version)
	}
}

func TestWindowUpdaterUsesProductionFetcher(t *testing.T) {
	_, logger := logtest.New(t)
	cfg, fetch, _, err := productionWindowUpdaterConfig(logger, coordinator.NewLaunchProbe())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = newWindowUpdater("/tmp/nocx", t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if fetch.HTTPClient == nil {
		t.Fatal("production constructor stored nil HTTP client")
	}
}

// Through the SAME constructor as ServiceStartup, force a real HTTP exchange.
// An unsigned response must not be confused with an up-to-date result (nil,nil).
func TestWindowUpdaterRefusesUnsignedManifest(t *testing.T) {
	old := version.Version
	version.Version = "0.8.0"
	defer func() { version.Version = old }()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/manifest.json" {
			_, _ = w.Write([]byte(`{"version":"0.7.2"}`))
			return
		}
		_, _ = w.Write([]byte("bm90IGEgc2lnbmF0dXJl"))
	}))
	defer server.Close()
	_, logger := logtest.New(t)
	cfg, fetch, _, err := productionWindowUpdaterConfig(logger, coordinator.NewLaunchProbe())
	if err != nil {
		t.Fatal(err)
	}
	updater, _, err := newWindowUpdater("/tmp/nocx", t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	fetch.BaseURL = server.URL
	if _, err := updater.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("unsigned manifest accepted: %v", err)
	}
	if calls != 2 {
		t.Fatalf("HTTP calls=%d, want manifest and signature", calls)
	}
}

func TestWindowUpdaterReportsFetchFailure(t *testing.T) {
	old := version.Version
	version.Version = "0.8.0"
	defer func() { version.Version = old }()
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	_, logger := logtest.New(t)
	cfg, fetch, _, err := productionWindowUpdaterConfig(logger, coordinator.NewLaunchProbe())
	if err != nil {
		t.Fatal(err)
	}
	updater, _, err := newWindowUpdater("/tmp/nocx", t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	fetch.BaseURL = server.URL
	if _, err := updater.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("HTTP failure accepted: %v", err)
	}
}
