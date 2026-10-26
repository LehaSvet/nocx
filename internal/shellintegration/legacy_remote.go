package shellintegration

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
)

// legacyRetirementStub is deliberately not a refreshed script: an interrupted
// retirement leaves either the old file or an inert file, never a third,
// partially copied integration. The unchanged rc gate can safely source it.
// Keep provenance in the on-disk file so it can be recognised without running
// it. VERSION is untouched: an older local install may keep these targets
// inert until explicitly reinstalled or upgraded.
func legacyRetirementStub(generation string) string {
	return "# nocx installer loader neutralised by generation " + generation + "\n" +
		"# Injected startup owns integration; this legacy gate target is inert.\n" +
		":\n"
}

// retireLegacyScripts touches only the installer's fixed targets, never rc
// files. Each replacement is a synced temporary file followed by an atomic
// rename, so a concurrent source reads one whole inode. Failure is reported
// and the next publish retries, including when its bundle is already current.
// A partial set of replacements is safe: every replaced target is inert and
// the injected startup latch also blocks any old target left behind.
func (p *Publisher) retireLegacyScripts(generation string) error {
	// Share the publisher's destination lock: its staging sweep must never
	// remove a retirement temp file between our write and atomic rename.
	release, err := p.acquireLock()
	if err != nil {
		return err
	}
	defer release()
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	data := []byte(legacyRetirementStub(generation))
	for _, name := range names {
		target := p.join(name)
		info, err := p.fs.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return boundaryErr("lstat", target, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &SymlinkError{Path: target}
		}
		if !info.Mode().IsRegular() {
			return &PublishError{Op: "retire", Path: target, Err: fmt.Errorf("not a regular file")}
		}
		current, err := p.fs.ReadFile(target)
		if err != nil {
			return boundaryErr("read", target, err)
		}
		if bytes.Equal(current, data) {
			continue
		}
		staged := p.join(tmpName, "retired-"+name+"."+newNonce())
		if err := p.replaceLegacyTarget(staged, target, data); err != nil {
			return err
		}
	}
	return nil
}

func (p *Publisher) replaceLegacyTarget(staged, target string, data []byte) error {
	// Also remove staging on write/sync/close failure, not only rename failure.
	defer func() {
		if err := p.fs.Remove(staged); err != nil && !errors.Is(err, fs.ErrNotExist) {
			p.log.Warn("shellintegration: retired loader staging not cleared", "err", err)
		}
	}()
	if err := p.writeFile(staged, data, 0o600); err != nil {
		return err
	}
	if err := p.fs.Rename(staged, target); err != nil {
		return boundaryErr("rename", target, err)
	}
	return p.syncDir(p.root)
}
