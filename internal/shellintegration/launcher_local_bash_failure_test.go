package shellintegration

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type localRcfileFault struct {
	events []string
	body   strings.Builder
	fail   string
	closed bool
}

var errLocalRcfileProbe = errors.New("rcfile I/O refused")

func (f *localRcfileFault) Name() string { return "empty-script" }
func (f *localRcfileFault) Write(p []byte) (int, error) {
	f.events = append(f.events, "write")
	if f.fail == "write" {
		return 0, errLocalRcfileProbe
	}
	if f.fail == "short" {
		return 0, nil
	}
	return f.body.Write(p)
}

func (f *localRcfileFault) Seek(offset int64, whence int) (int64, error) {
	f.events = append(f.events, "seek")
	if f.fail == "seek" {
		return 0, errLocalRcfileProbe
	}
	if offset != 0 || whence != io.SeekStart {
		return 0, errors.New("wrong rewind")
	}
	return 0, nil
}

func (f *localRcfileFault) Close() error {
	f.closed = true
	f.events = append(f.events, "close")
	return nil
}

// Assert the whole interval: no script bytes exist while a name exists,
// and every failure closes the descriptor without allowing the next step.
func TestLocalBashRcfilePreparationOrdersAndUnwindsIO(t *testing.T) {
	for _, fail := range []string{"", "unlink", "write", "short", "seek"} {
		t.Run("fail-"+fail, func(t *testing.T) {
			f := &localRcfileFault{fail: fail}
			unlink := func(name string) error {
				f.events = append(f.events, "unlink")
				if name != f.Name() || f.body.Len() != 0 {
					t.Fatal("script was written before its name was removed")
				}
				if fail == "unlink" {
					return errLocalRcfileProbe
				}
				return nil
			}
			err := prepareLocalRcfile(f, unlink, "private-script")
			wantEvents := map[string]string{
				"":       "unlink,write,seek",
				"unlink": "unlink,close",
				"write":  "unlink,write,close",
				"short":  "unlink,write,close",
				"seek":   "unlink,write,seek,close",
			}[fail]
			if got := strings.Join(f.events, ","); got != wantEvents {
				t.Fatalf("I/O order=%s, want %s", got, wantEvents)
			}
			if fail == "" {
				if err != nil || f.closed || f.body.String() != "private-script" {
					t.Fatalf("prepared file not ready for spawn: closed=%v err=%v", f.closed, err)
				}
			} else {
				want := errLocalRcfileProbe
				if fail == "short" {
					want = io.ErrShortWrite
				}
				if !errors.Is(err, want) || !f.closed {
					t.Fatalf("failure did not unwind: closed=%v err=%v", f.closed, err)
				}
			}
		})
	}
}
