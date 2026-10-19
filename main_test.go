package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/shady2k/nocx/internal/coordinator"
	"github.com/shady2k/nocx/internal/log/logtest"
)

func TestResolveBackendReportsSuccessAndFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		w := &WailsApp{
			logger: slog.Default(),
			launchCoordinatorFn: func(context.Context, string) (coordinator.Launch, error) {
				return coordinator.Launch{Hello: coordinator.Hello{
					WSAddress: "127.0.0.1:4321",
					WSToken:   "capability-token",
				}}, nil
			},
		}

		got := w.ResolveBackend()
		want := BackendResolution{
			OK:    true,
			Host:  "127.0.0.1",
			Port:  4321,
			Token: "capability-token",
		}
		if got != want {
			t.Fatalf("ResolveBackend() = %#v, want %#v", got, want)
		}
	})

	t.Run("failure", func(t *testing.T) {
		cause := errors.New("server could not start")
		failure := coordinator.NewLaunchFailure(
			coordinator.FailureNotReady,
			"The backend is not ready.",
			"Retry the launch.",
			cause,
		)
		w := &WailsApp{
			logger: slog.Default(),
			launchCoordinatorFn: func(context.Context, string) (coordinator.Launch, error) {
				return coordinator.Launch{}, failure
			},
		}

		got := w.ResolveBackend()
		want := BackendResolution{
			Kind:    string(coordinator.FailureNotReady),
			Message: failure.Message,
			Remedy:  failure.Remedy,
		}
		if got != want {
			t.Fatalf("ResolveBackend() = %#v, want %#v", got, want)
		}
	})
}

func TestResolveBackendReturnsHeldStartupFailureBeforeRetry(t *testing.T) {
	failure := coordinator.NewLaunchFailure(
		coordinator.FailureProfileUnusable,
		"The profile directories could not be used.",
		"Check the profile directories, then retry.",
		nil,
	)
	attempts := 0
	w := &WailsApp{
		logger:        slog.Default(),
		launchFailure: failure,
		launchCoordinatorFn: func(context.Context, string) (coordinator.Launch, error) {
			attempts++
			return coordinator.Launch{Hello: coordinator.Hello{
				WSAddress: "127.0.0.1:4322",
				WSToken:   "new-token",
			}}, nil
		},
	}

	if got := w.ResolveBackend(); got.Kind != string(coordinator.FailureProfileUnusable) {
		t.Fatalf("first ResolveBackend() = %#v, want held failure", got)
	}
	if attempts != 0 {
		t.Fatalf("first ResolveBackend launched %d times, want 0", attempts)
	}
	if got := w.ResolveBackend(); !got.OK || got.Port != 4322 || got.Token != "new-token" {
		t.Fatalf("retry ResolveBackend() = %#v, want successful retry", got)
	}
	if attempts != 1 {
		t.Fatalf("retry launched %d times, want 1", attempts)
	}
}

// A notice is only a notice once somebody has seen it.
//
// The launcher announces what it did BEFORE it can show anything: a dialog
// needs a window, and at that moment in startup there is none (Wails creates
// the window after the services start). showNotices therefore has to survive
// having nowhere to put the notice — and it did not: it drained the recorder
// first and then returned early, so the one message an upgrade depends on was
// dropped on exactly the launch that has to deliver it (nocx-1ebrs.6).
func TestShowNoticesKeepsWhatItCannotShow(t *testing.T) {
	w := &WailsApp{logger: logtest.Slog(t)}
	w.notices.Announce(coordinator.Notice{
		Kind:    coordinator.NoticeSessionsLost,
		Message: "The nocx backend that was already running has been stopped and replaced.",
	})

	// No Wails application in this process: this is the startup case, and it
	// is also the only way to see the difference between "shown" and "thrown
	// away".
	w.showNotices()

	if held := w.notices.drain(); len(held) != 1 {
		t.Fatalf("after a show with no window, %d notice(s) are still waiting; want 1 — "+
			"a notice that is neither shown nor kept is a notice nobody will ever see", len(held))
	}
}

// recordingShell is a shellWindow that remembers what it was asked to do, in
// order. The order is the point: a dialog raised before the window is the
// defect, so "which call came first" is the assertion.
type recordingShell struct {
	calls   []string
	notices []coordinator.Notice
}

func (s *recordingShell) Show() { s.calls = append(s.calls, "show") }

func (s *recordingShell) Present(n coordinator.Notice) {
	s.calls = append(s.calls, "present")
	s.notices = append(s.notices, n)
}

// The bead's regression test, at the seam the launch resolves on.
//
// A launch that replaced an incompatible coordinator has to end with a window —
// that is what the person was left without, on the one launch every existing
// user takes — and the one sentence explaining what happened has to reach it,
// after the window and not before.
func TestAReplacementEndsWithAWindowAndThenTheNotice(t *testing.T) {
	shell := &recordingShell{}
	w := &WailsApp{
		logger: logtest.Slog(t),
		shell:  shell,
		launchCoordinatorFn: func(context.Context, string) (coordinator.Launch, error) {
			return coordinator.Launch{
				Hello:    coordinator.Hello{WSAddress: "127.0.0.1:4323", WSToken: "token"},
				Spawned:  true,
				Replaced: true,
			}, nil
		},
	}
	w.notices.Announce(coordinator.Notice{
		Kind:    coordinator.NoticeSessionsLost,
		Message: "The nocx backend that was already running has been stopped and replaced.",
	})

	if got := w.ResolveBackend(); !got.OK {
		t.Fatalf("ResolveBackend() = %#v, want a resolved backend after a replacement", got)
	}

	if len(shell.calls) != 2 || shell.calls[0] != "show" || shell.calls[1] != "present" {
		t.Fatalf("the shell was asked for %v; want [show present] — the window first, then "+
			"what the launcher did", shell.calls)
	}
	if len(shell.notices) != 1 || shell.notices[0].Kind != coordinator.NoticeSessionsLost {
		t.Fatalf("the shell was given %#v; want the sessions-lost notice", shell.notices)
	}
	if left := w.notices.drain(); len(left) != 0 {
		t.Fatalf("%d notice(s) still recorded after they were shown; want 0", len(left))
	}
}
