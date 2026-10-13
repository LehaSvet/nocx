package session

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDaemonLifecycleDeadlineAdmissionWinsAtomically(t *testing.T) {
	drained := make(chan struct{})
	l := newDaemonLifecycle(time.Hour, func() { close(drained) })
	release, err := l.admit()
	if err != nil {
		t.Fatal(err)
	}
	l.expire()
	if _, err := l.admit(); !errors.Is(err, ErrDraining) {
		t.Fatalf("admission after deadline = %v, want draining", err)
	}
	l.sessionStarted()
	release()
	select {
	case <-drained:
		t.Fatal("daemon drained despite winning admission")
	default:
	}
	l.sessionEnded()
	select {
	case <-drained:
	default:
		t.Fatal("last session did not drain daemon")
	}
	l.stop()
}

func TestDaemonLifecycleScheduledGraceExpires(t *testing.T) {
	drained := make(chan struct{})
	var scheduled time.Duration
	var fire func()
	l := newDaemonLifecycleWithTimer(time.Hour, func() { close(drained) }, func(d time.Duration, f func()) lifecycleTimer {
		scheduled, fire = d, f
		return inertLifecycleTimer{}
	})
	if scheduled != time.Hour {
		t.Fatalf("scheduled grace = %s, want %s", scheduled, time.Hour)
	}
	if fire == nil {
		t.Fatal("startup grace did not schedule its expiry")
	}
	select {
	case <-drained:
		t.Fatal("daemon drained before scheduled grace fired")
	default:
	}
	fire()
	if _, err := l.admit(); !errors.Is(err, ErrDraining) {
		t.Fatalf("post-grace admission = %v, want draining", err)
	}
	select {
	case <-drained:
	default:
		t.Fatal("empty daemon did not drain")
	}
	l.stop()
}

type inertLifecycleTimer struct{}

func (inertLifecycleTimer) Stop() bool { return true }

func TestDaemonLifecycleFailedAdmittedSpawnReleasesReservation(t *testing.T) {
	drained := make(chan struct{})
	var fire func()
	l := newDaemonLifecycleWithTimer(time.Hour, func() { close(drained) }, func(_ time.Duration, f func()) lifecycleTimer {
		fire = f
		return inertLifecycleTimer{}
	})
	if fire == nil {
		t.Fatal("startup grace did not schedule its expiry")
	}
	finishAdmission, err := l.admit()
	if err != nil {
		t.Fatalf("admit spawn: %v", err)
	}
	// The scheduled deadline may arrive while an already-admitted spawn is
	// failing. Its deferred reservation release must be what lets retirement run.
	fire()
	select {
	case <-drained:
		t.Fatal("daemon drained while admitted spawn was still in flight")
	default:
	}
	finishAdmission()
	select {
	case <-drained:
	default:
		t.Fatal("failed admitted spawn kept daemon admission reserved")
	}
	if _, err := l.admit(); !errors.Is(err, ErrDraining) {
		t.Fatalf("admission after failed spawn and grace = %v, want draining", err)
	}
	l.stop()
}

func TestDaemonLifecycleDeadlineRacesAdmission(t *testing.T) {
	for i := 0; i < 100; i++ {
		var mu sync.Mutex
		admitted, refused, drained := false, false, false
		l := newDaemonLifecycle(time.Hour, func() { mu.Lock(); drained = true; mu.Unlock() })
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; l.expire() }()
		go func() {
			defer wg.Done()
			<-start
			release, err := l.admit()
			if err != nil {
				refused = true
				return
			}
			admitted = true
			l.sessionStarted()
			release()
		}()
		close(start)
		wg.Wait()
		mu.Lock()
		d := drained
		mu.Unlock()
		if admitted == refused {
			t.Fatalf("iteration %d: admitted=%t refused=%t", i, admitted, refused)
		}
		if admitted && d {
			t.Fatalf("iteration %d: acknowledged session but daemon drained", i)
		}
		if admitted {
			l.sessionEnded()
		}
		if refused && !d {
			t.Fatalf("iteration %d: refused admission but daemon did not drain", i)
		}
		l.stop()
	}
}
