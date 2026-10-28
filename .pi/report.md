# nocx-1ebrs.66.1.9 report

The Updates section now says Linux AppImage installs can update themselves in nocx. It also says deb and rpm installs update through their package manager, which is a different install from the AppImage. The section remains on the existing Updates page.

Red: `flock /tmp/nocx-sky.lock -c "nocx-sky-test /home/dev/.herdr/worktrees/nocx/w-n669 'cd frontend && npm exec -- vitest run src/updates-section.test.tsx'"` — command exit 1. The intended assertion failed because the old install-only card rendered no AppImage self-update text.

Green: `flock /tmp/nocx-sky.lock -c "nocx-sky-test /home/dev/.herdr/worktrees/nocx/w-n669 'cd frontend && npm exec -- vitest run src/updates-section.test.tsx'"` — command exit 0; all 3 tests passed.
