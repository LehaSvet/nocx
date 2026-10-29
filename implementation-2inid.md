# nocx-2inid implementation evidence

Status: implementation in progress; no renderer red/green claim yet.
Baseline: `6062102f0`, branch `fix/nocx-2inid-colrv1`.

## Package and loader policy

The package pin is jammy-updates `libfreetype6` version
`2.11.1+dfsg-1ubuntu0.4` for amd64. Its SHA256 is
`3728210ef44a7a51b3824d5c499f6fbd428584a42a65aee15794a4264c8c173a`,
independently confirmed against jammy-updates/main/binary-amd64/Packages.gz.
The library ELF SHA256 before packaging is
`c338538f8d90eb85217e8525ab363e68e79b46d3c8acc0b6824190e032687e5f`.

`scripts/appimage/stage-freetype.sh` downloads and checks that exact package,
checks control metadata, copies the regular library under its SONAME, and
loads the copied file to assert FreeType 2.11.1. It stages the package's entire
document directory, including copyright and changelog, and writes package URL,
version, archive hash and copied ELF hash to `nocx-provenance.txt`.
Packaging reads the library and notices back out of the final AppImage.

Normal loading uses nocx's `$ORIGIN/../lib` and WebKit's `$ORIGIN` RUNPATH.
A review rejected prepending bundle-wide `LD_LIBRARY_PATH` because it leaks to
all terminal descendants. No AppRun environment change is retained. Explicit
user `LD_LIBRARY_PATH`/`LD_PRELOAD` overrides are unsupported; process-map
checks reject an outside-image FreeType in the measured release case.

FreeType depends on PNG, Brotli, zlib and libc. PNG and Brotli are already in
the AppImage. zlib remains host-provided, as before, under its stable SONAME;
this change does not bundle libc or unrelated host fontconfig/HarfBuzz.

## Controlled artifact preparation

A fresh directory `/tmp/nocx-2inid-rehearsal-sfbfkugr` holds a new full extract
of the released 0.8.0 AppImage. No Sky persisted workdir was used as a baseline.
The original release SHA256 is
`99054eaf8e8eb93fa3d61f89aa77c7c13d1b365a76209ef4d438117fb1f2370f`.
The candidate is the same released binaries, with only matched FreeType,
notices/provenance and AppRun loader hook changed. It is a controlled repair
rehearsal, not a newly built release artifact.

The exact `stage-freetype.sh` ran in `ubuntu:22.04` Docker (exit 0).
It printed `FreeType ABI checked: ... version=(2, 11, 1)`.
The pinned appimagetool 1.9.1 and type2 runtime 20251108 repacked the extracted
AppDir into `repaired.AppImage` in Docker (exit 0).
Logs: `/tmp/nocx-2inid-stage.log`, `/tmp/nocx-2inid-repack.log`.

## First measured renderer results (not an accepted red/green)

The current Arch host printed FreeType `2.15.0-1`, fontconfig
`2:2.18.3-2`, HarfBuzz `15.0.0-1`. The pinned static font's U+1F600
`u1F600` traverses PaintRadialGradient (format 6) with **three stops**.
The original AppImage with host FreeType 2.15.0 survived this paint and
responded after a snapshot. Its DOM and canvas each contained 2107 chromatic
pixels. Main and WebProcess maps both printed `/usr/lib/libfreetype.so.6.20.7`.
This does **not** reproduce the reported assertion.

The controlled repaired artifact survived the same probe, then the actual
AppImage window reached a terminal, painted title/output, accepted another
keyboard command and wrote a response file. Main and WebProcess maps both
printed `/work/squashfs-root/usr/lib/libfreetype.so.6` version 2.11.1 before
and after. An actual terminal child reported empty `LD_LIBRARY_PATH`.
The `none` case passed. Its deletion mutation also survived, so the overall
gate correctly exited 1 (`mutation survived the verified gradient`), not 0.
The original `unbundled-baseline` case exited 1 for the same reason.
Commands and exact own statuses:

- `NOCX_APPIMAGE_VERIFY_IMAGE=nocx-2inid-arch:verified-host
NOCX_APPIMAGE_VERIFY_EVIDENCE=/tmp/nocx-2inid-rehearsal-sfbfkugr/evidence
scripts/appimage/verify-appimage.sh
/tmp/nocx-2inid-rehearsal-sfbfkugr/repaired.AppImage none no-freetype`
  exited **1**. Log `/tmp/nocx-2inid-gate2.log`.
- Same command against `released.AppImage unbundled-baseline` exited **1**.
  Log `/tmp/nocx-2inid-baseline.log`.

Screenshots and full per-case logs are in the rehearsal directory's
`evidence/none` and `baseline-evidence/unbundled-baseline` folders. The model
could not inspect the PNGs visually because it lacks vision; no claim of a
manual glyph-vs-tofu image review is made. The shipped-engine snapshot's
chromatic-pixel assertion did pass.

The first attempt exited 1 before any renderer work: importing host Pillow
under the probe's command-scoped bundle loader path bound an incompatible
JPEG library into host TIFF. The probe now uses the shipped Cairo image C API
instead; it no longer imports Pillow. This was harness failure, not product red.

Narrow hook contract tests ran through `nocx-sky-test` (2 CPU / 4 GB), twice,
with command exit **0**, two unittest assertions each time. The retained hook
has no loader environment export. No Go/vitest/Playwright suite ran on the VM.

## Accepted bounded regression rehearsal

The all-three fixture is essential. It includes:

| Text          | Glyph     | COLRv1 gradient graph              |
| ------------- | --------- | ---------------------------------- |
| U+26A0 + FE0F | `uni26A0` | two linear gradients, 2 stops each |
| U+1F308       | `u1F308`  | no gradient; solid-colour control  |
| U+1F600       | `u1F600`  | one radial gradient, 3 stops       |

The original release reproduces the exact assertion on **both** Arch FreeType
2.15.0 and Ubuntu 24.04 FreeType 2.13.2. The original main/WebProcess maps name
host `libfreetype.so.6.20.7` (Arch) and `libfreetype.so.6.20.1` (noble).
The failing line contains all of `colrv1_configure_skpaint`, `ColorStop` and
`Assertion '__n < this->size()' failed`; the probe returns **1** and reports
`WEB_PROCESS_TERMINATED reason=0`. The negative-control harness returns **0**
only after checking that exact line and the WebProcess loader evidence.

The controlled repaired artifact SHA256 is
`96a54550a1adaf1b974310446f90471365a3ce82e88280f8ffccb4ed14a8adf1`.
It uses unchanged released executable/WebKit/helper bytes. Its main and
WebProcess both map `AppDir/usr/lib/libfreetype.so.6` version **2.11.1**
before/after painting and after the actual AppImage input response.
`ldd -r` finds no missing library/undefined symbol for nocx, WebProcess,
modern host fontconfig or HarfBuzz using the bundle pairing.

The shipped-engine DOM comparison uses each span's own
`getBoundingClientRect()` after fonts and two animation frames. On noble:

| Text     | Crop x/y/w/h  | Colour pixels | Hue buckets | Mean saturation | Tofu pixels/hues/saturation |
| -------- | ------------- | ------------- | ----------- | --------------- | --------------------------- |
| warning  | 0/0/200/160   | 4118          | 2           | 0.80659         | 0/0/0                       |
| rainbow  | 200/0/200/160 | 4914          | 17          | 0.85527         | 0/0/0                       |
| grinning | 400/0/200/160 | 5438          | 4           | 0.80562         | 0/0/0                       |

The control replaces the span's font with monospace and its content with
missing scalar U+10FFFF on the same page, then snapshots the same rectangles.
Canvas independently paints the glyph set; the grinning radial-gradient and
rainbow columns require colour pixels. U+26A0 canvas is allowed text-style
presentation (DOM warning remains mandatory colour). A JS response after the
snapshot and actual window command after title/output both succeed.
The actual window's terminal child writes empty `LD_LIBRARY_PATH`.

Final exact commands (snapshot paths freeze scripts against concurrent edits):

- `NOCX_APPIMAGE_VERIFY_IMAGE=nocx-2inid-arch:verified-host
NOCX_APPIMAGE_VERIFY_EVIDENCE=/tmp/nocx-2inid-rehearsal-sfbfkugr/arch-final-evidence
/tmp/nocx-2inid-gate-snapshot-gln0ayvd/appimage/verify-appimage.sh
/tmp/nocx-2inid-rehearsal-sfbfkugr/repaired.AppImage`
  exited **0**, all five default cases. Full log
  `/tmp/nocx-2inid-arch-final.log`.
- Same gate with `NOCX_APPIMAGE_VERIFY_IMAGE=nocx-2inid-noble:verified-host`
  and `none no-freetype` exited **0**. Full log
  `/tmp/nocx-2inid-noble-final3.log`.
- The original `released.AppImage unbundled-baseline` on Arch exited **0**
  (expected exact product failure); log
  `/tmp/nocx-2inid-arch-baseline-three.log`.
- The same original baseline on noble exited **0**; log
  `/tmp/nocx-2inid-noble-baseline2.log`.

Deleting only bundled FreeType reproduces the original exact assertion on
both hosts. Removing either helper or restoring its old absolute path also
fails for each existing expected helper-spawn reason. No generic timeout or
unrelated abort is accepted as a negative pass.

Docker images used: Arch dependency snapshot
`sha256:24c23ffe09cef8eb99033d032b24d501b1dd304f3368a832af4bcc65b43d9f14`;
noble dependency snapshot
`sha256:bce3098bdabee71f818320f3d723e2b4416f3b3232d050d13962de8874422932`.
Arch base was `archlinux:latest`; noble base was `ubuntu:24.04` digest
`sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852`.
Each run extracts a fresh artifact and uses a disposable HOME/fontconfig cache.

## Limits and discarded trial

The reporter's exact font, glyph, host loaded pathname and corrupted field are
still unknown. The all-three set reproduces the exact failure; it does not
attribute the crash to one glyph in isolation. A richer shared paint graph
exposes the mismatch, but its exact overwritten local field was not debugged.
The actual window screenshot has not been manually/semantically inspected;
input-response and process maps prove survival, while the separate shipped
renderer probe provides controlled glyph-paint evidence.

A bounded optional complex/variable trial is **not shipped**. Static rich/ZWJ
graphs reproduced the assertion, but the first variable graph scan ran for five
minutes without renderer output and was stopped. After a visited-set fix, the
variable default scenario rendered no colour in the PUA glyph and failed its
harness assertion; max coordinates were never reached. This produced no
accepted variable-font rendering evidence. Its code and wiring were removed.

No new nightly has been built/promoted, and no reporter-host retest occurred.
The full product suites belong to integration/CI and were not run by this
packaging worker. The P0 remains open for integration and next-nightly acceptance.

Final-source narrowed gate (complex/variable helper removed) reran
`none no-freetype` on Arch and exited **0**. Log
`/tmp/nocx-2inid-source-final.log`. This used a frozen final script snapshot
`/tmp/nocx-2inid-gate-final-2qz51wy0/appimage/verify-appimage.sh`.
A targeted independent review found no false-positive in the probe's
colour/control/response/exact-red checks. It identified the actual-window
semantic screenshot limit stated above. Fresh `--rm` containers are required;
the gate is not a supported persistent-container/workdir reuse interface.
