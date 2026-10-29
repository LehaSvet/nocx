#!/usr/bin/env python3
"""Measure the peak resident memory of one command's whole process tree.

    python3 scripts/measure-hook-memory.py .githooks/pre-commit

WHY THIS EXISTS. The pre-commit hook's frontend section is this machine's largest
memory spike, and the number that decided nocx-owbpb — two commits in two
worktrees running the typed eslint at once, 2.1 GB and 2.0 GB, 707 MB left on a
12 GB box every agent session shares — is that section's peak. It is also the one
number `time -v` cannot report: its "Maximum resident set size" is the DIRECT
child's, and the hook's direct child is bash at 4 MB while the memory lives two
levels down in node.

So this samples /proc every 25 ms and reports both figures: the peak of the whole
tree, which is what the machine pays, and each process's own peak, which names
the culprit. Run it with the command as its argument, from the directory the
command expects.

IT TAKES NO LOCK AND IT DOES NOT WAIT. A measurement taken while another gate is
running is a measurement of contention, so it says when it sees one rather than
silently moving the number.
"""

import json
import os
import subprocess
import sys
import time

SAMPLE_SECONDS = 0.025
HEAVY = ('eslint', 'tsc --noEmit', 'vitest', 'go test', 'golangci-lint', 'npm ci')


def _cmdline(pid):
    try:
        with open(f'/proc/{pid}/cmdline', 'rb') as f:
            return f.read().replace(b'\0', b' ').decode('utf8', 'replace').strip()
    except OSError:
        return ''


def _rss_kb(pid):
    try:
        with open(f'/proc/{pid}/status') as f:
            for line in f:
                if line.startswith('VmRSS:'):
                    return int(line.split()[1])
    except OSError:
        pass
    return 0


def _ppid(pid):
    try:
        with open(f'/proc/{pid}/stat') as f:
            data = f.read()
        # The command name is parenthesised and may contain spaces, so the fields
        # after it are found from the LAST ')', not by splitting the whole line.
        return int(data[data.rindex(')') + 2:].split()[1])
    except (OSError, ValueError):
        return 0


def heavy_neighbours(root_pid):
    """Processes outside this run that compete for the same memory."""
    out = []
    for pid in os.listdir('/proc'):
        if not pid.isdigit() or int(pid) == root_pid:
            continue
        line = _cmdline(pid)
        if 'measure-hook-memory' in line or 'nocx-sky-test' in line:
            continue
        if any(k in line for k in HEAVY) and ('node_modules' in line or 'worktrees/nocx' in line):
            out.append(line[:120])
    return out


def main(argv):
    cmd = argv[1:]
    if cmd and cmd[0] == '--':
        cmd = cmd[1:]
    if not cmd:
        print(__doc__.strip().splitlines()[2].strip(), file=sys.stderr)
        return 2

    started = time.time()
    proc = subprocess.Popen(cmd)
    for busy in heavy_neighbours(proc.pid):
        print(f'NOTE: another heavy gate is running: {busy}', file=sys.stderr)

    peak_tree_kb = 0
    peak_by_pid = {}
    peak_tree_top = []
    while proc.poll() is None:
        pids = [p for p in os.listdir('/proc') if p.isdigit()]
        parents = {p: _ppid(p) for p in pids}
        tree = {str(proc.pid)}
        grew = True
        while grew:
            grew = False
            for pid, parent in parents.items():
                if str(parent) in tree and pid not in tree:
                    tree.add(pid)
                    grew = True
        total = 0
        alive = []
        for pid in tree:
            kb = _rss_kb(pid)
            if not kb:
                continue
            total += kb
            alive.append((kb, pid, _cmdline(pid)[:120]))
            if kb > peak_by_pid.get(pid, (0, ''))[0]:
                peak_by_pid[pid] = (kb, _cmdline(pid)[:120])
        if total > peak_tree_kb:
            peak_tree_kb = total
            peak_tree_top = sorted(alive, reverse=True)[:5]
        time.sleep(SAMPLE_SECONDS)

    code = proc.wait()
    elapsed = time.time() - started
    peaks = sorted(peak_by_pid.values(), reverse=True)
    result = {
        'command': ' '.join(cmd),
        'exit': code,
        'seconds': round(elapsed, 1),
        'peak_tree_mb': round(peak_tree_kb / 1024, 1),
        'peak_tree_at': [[round(kb / 1024, 1), c] for kb, _pid, c in peak_tree_top],
        'peak_per_process_mb': [[round(kb / 1024, 1), c] for kb, c in peaks[:10]],
    }
    print(json.dumps(result, indent=2))
    return code


if __name__ == '__main__':
    sys.exit(main(sys.argv))
