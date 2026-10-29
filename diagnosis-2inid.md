# Diagnosis: nocx-2inid — Linux COLRv1 renderer abort

Status: diagnosis only. No product fix, commit, or test pass is claimed.
Worktree: `/home/dev/.herdr/worktrees/nocx/w-2inid`, branch
`fix/nocx-2inid-colrv1`, baseline `6062102f0`.

## Result and evidence limits

**The released packaging permits an incompatible FreeType COLRv1 ABI pairing.**
The bundled WebKit was compiled with FreeType 2.11.1 headers. The AppImage
carries no FreeType library, so it uses the host's. FreeType 2.13 changed the
experimental COLRv1 structs without changing `libfreetype.so.6`.
That is a strong, source-supported explanation of this exact abort, independently
reproduced by another AppImage project. **I have not reproduced nocx's abort in
our container.** Capacity prevented a renderer process from starting. The exact
reporter's font, glyph, resolved FreeType pathname/version, and corrupt local
field are unproven. Do not call this a measured nocx red/green result yet.

## 1. What the released AppImage actually carries

Downloaded with (exit 0):

```
gh release download v0.8.0 --repo shady2k/nocx \
  --pattern nocx-0.8.0-linux-amd64.AppImage \
  --dir /tmp/nocx-2inid-artifact --clobber
sha256sum /tmp/nocx-2inid-artifact/nocx-0.8.0-linux-amd64.AppImage
```

The second command exits 0 and prints
`99054eaf8e8eb93fa3d61f89aa77c7c13d1b365a76209ef4d438117fb1f2370f`,
identical to the GitHub release asset digest. Asset URL:
https://github.com/shady2k/nocx/releases/download/v0.8.0/nocx-0.8.0-linux-amd64.AppImage

Executed the image's `--appimage-extract` with **no pattern** in
`/tmp/nocx-2inid-artifact` (exit 0). Python `Path.rglob('*')` across the complete
`squashfs-root` finds only `usr/lib/girepository-1.0/freetype2-2.0.typelib` under
names containing `freetype`; there is no FreeType ELF or symlink anywhere in the
extracted archive. This covers more than the initial `usr/lib/*` extraction.

The renderer engine lives in `usr/lib/libwebkit2gtk-4.1.so.0` (90,003,720 bytes),
with Skia inside it; there is no separately loaded Skia library. The bundle also
ships its `WebKitWebProcess` and `WebKitNetworkProcess` in
`usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/`.

`readelf -n .../libwebkit2gtk-4.1.so.0` exits 0 and reports build ID
`5453dc26598bc698bee8376a0266f979a653dac7`.
The library's printable strings contain **the exact** `colrv1_configure_skpaint`
`ColorStop` vector `operator[]` specialization and
`/usr/include/c++/12/bits/stl_vector.h` from the report.
`objdump -d --start-address=0xdf4f50 --stop-address=0xdf4f80 <library>` exits 0:
`webkit_get_major_version` returns 2, minor returns 0x32 (50), micro returns 4.

The successful nightly that supplied the promoted 0.8.0 asset is
https://github.com/shady2k/nocx/actions/runs/40922572094 .
`gh run view 40922572094 --repo shady2k/nocx --log` exits 0; saved at
`/tmp/nocx-2inid-nightly.log`. Lines 1476–1477 and 1852–1857 record apt installing
`libwebkit2gtk-4.1-0` / `-dev` **2.50.4-0ubuntu0.22.04.1** from jammy-updates.
This is an **Ubuntu prebuilt**, not our WebKit build and not proof of a Debian
bookworm builder merely because a GCC 12 assertion path appears.

Current build: `.github/workflows/release-build.yml:897-932` uses ubuntu-22.04,
`apt-get install ... libwebkit2gtk-4.1-dev` without a version pin.
Lines 1003–1032 build nocx and its coordinator, not WebKit.
`scripts/appimage/package-appimage.sh:88-92` copies distro helper binaries;
`:175-179` deploys dependency closure; `:196-199` patches the library's helper
lookup path. `br show nocx-azxe.7` exits 0 and describes this same prebuilt
bundling. ADR-0035:135–137 explicitly rejects owning WebKit security builds.

**Loader paths:** `readelf -d` exits 0 for both ELFs. `usr/bin/nocx` has
RUNPATH `$ORIGIN/../lib`; **libwebkit itself has RUNPATH `$ORIGIN`** and
DT_NEEDED `libfreetype.so.6`. `AppRun` sources the GTK hook and nocx cwd hook;
the GTK hook changes resource paths, not a hidden FreeType directory. With no
bundled FreeType, the renderer must resolve it outside the image. Its actual
path/version on the colleague's host is not available. No `ldd` runtime result
on that host is claimed.

## 2. Compile headers and exact upstream mechanism

Actual Ubuntu amd64 package build record:
https://launchpad.net/~ubuntu-security-proposed/+archive/ubuntu/ppa/+build/32114182

Its buildinfo (HTTP 200, read directly, saved to
`/tmp/nocx-2inid-buildinfo.txt`):
https://launchpad.net/~ubuntu-security-proposed/+archive/ubuntu/ppa/+build/32114182/+files/webkit2gtk_2.50.4-0ubuntu0.22.04.1_amd64.buildinfo

`Installed-Build-Depends` names:

```
libfreetype-dev (= 2.11.1+dfsg-1ubuntu0.3)
libfreetype6 (= 2.11.1+dfsg-1ubuntu0.3)
libfreetype6-dev (= 2.11.1+dfsg-1ubuntu0.3)
gcc-12 (= 12.3.0-1ubuntu1~22.04.2)
```

This is direct build provenance, stronger than inference from jammy's defaults.

FreeType's primary changes:

- https://github.com/freetype/freetype/commit/117df36b4c3a5f7850a3789e92ce54d9341673ff
  (2022-06-22): `FT_ColorStop.stop_offset` changes from `FT_F2Dot14` to `FT_Fixed`.
- https://github.com/freetype/freetype/commit/3414fef74f9dd1858aca521f93ebadca9ab36f96
  (2022-07-08): adds `FT_ColorStopIterator.read_variable`.
- Old definitions and experimental-ABI warning:
  https://github.com/freetype/freetype/blob/VER-2-11-1/include/freetype/ftcolor.h#L545-L611
- New definitions:
  https://github.com/freetype/freetype/blob/VER-2-13-0/include/freetype/ftcolor.h#L549-L614

`FT_F2Dot14` is a short and `FT_Fixed` a long (a **64-bit C storage object on
Linux amd64**, despite its 16.16 fixed-point meaning). This changes the struct's
size/alignment/field offsets. The iterator gains another field. Do not describe
this as merely a changed 32-bit number. The prebuilt allocates old-layout local
objects; the newer library reads/writes new-layout ones through the same symbols.
That can overwrite adjacent stack data and make the stop count/index invalid.
**The exact corrupted field in this nocx binary has not been inspected in a
debugger.** “Reads a count from the wrong byte” is a mechanism family, not a
measured field-level trace here.

The exact Skia crash site in our vendored engine:
https://github.com/WebKit/WebKit/blob/webkitgtk-2.50.4/Source/ThirdParty/skia/src/ports/SkFontHost_FreeType_common.cpp#L607-L640

It resizes the vector to `num_color_stops`, calls `FT_Get_Colorline_Stops` into
local `FT_ColorStop` and copied iterator, then uses `current_color_stop - 1`
without bounds checking. Lines 103–107 handle the stop representation by
**compile-time sizeof**, not runtime ABI. No runtime ABI guard protects this.

An exact guard exists in a **rust-skia fork**, not a verified WebKit release:

- https://github.com/rust-skia/rust-skia/issues/1345
- https://github.com/rust-skia/skia/commit/fb635bc3c03e1cd085dfedbd8b11ab19e4d8d4e5
  (2026-10-10), “Use COLRv1 only when the FreeType runtime matches the header ABI”.
- Integrated by https://github.com/rust-skia/rust-skia/commit/b27fffd59af238dcd5399f52308074ede5b3b673
  (m156.2). It compares header/runtime sides of the 2.13 boundary and declines
  COLRv1 classification on mismatch.

Checked WebKitGTK tags 2.50.6, 2.52.6 and 2.54.2 still have the unguarded path;
no confirmed upstream WebKitGTK fixed release or accepted WebKit/Ubuntu bug was
found. 2.54.2 is listed stable on 2026-10-23 at https://webkitgtk.org/releases/ .
Do not invent a fixed version or promise that waiting for one repairs this.
The fork guard is the concrete backport candidate that would avoid the unsafe
path, but it suppresses COLRv1 on incompatible runtimes; a compatible FreeType
pairing preserves colour rendering.

Independent exact-assertion application evidence (not our reproduction):
https://github.com/home-assistant/installer/pull/366 , merged 2026-10-21.
It reports jammy WebKit with host FreeType >=2.13 and COLRv1 font, and repairs
it by bundling FreeType. Reported failures: Fedora43 5/5 before vs 0/5 after;
NixOS 2/2 before vs 0/4 after. It also checked host HarfBuzz/fontconfig imports.

## 3. Which glyph / UI element

**Actual font and glyph remain unknown.** A host colour-emoji fallback is
plausible, not established. The requested source investigation finds no built-in
45-second emoji insertion.

- Pet sleep is a tempting timing match: `frontend/src/pets/pet.ts:214` sets
  `sleepAfter:45`; `:1250-1257` enters sleep after accumulated idle boredom.
  But `pets/pack.ts:112-116` maps sleep to PNG files and
  `pets/overlay.ts:855-857` paints CSS background sprites, not text. This cannot
  itself call FreeType for a sleep emoji. Running milestones are 5/20/60s
  (`pet.ts:919-921`), not 45s.
- Strongest requested UI candidate: arbitrary program **tab title**.
  `terminal-content.ts:5277`, `:7182-7189` accepts the title;
  `:2308-2309` forwards program/command/cwd title;
  `tab.tsx:324` renders it verbatim. UI font stack is platform sans fallback
  (`styles/themes/graphite.css:59`, `tab-strip.css:31,128`). No emoji family is
  pinned there. Terminal output itself and command text can also supply Unicode.
- Status chips are CSS spans/dots/rings, not emoji (`tab.tsx:307,317-322`,
  `styles/components/tab.css:318-372,403-410`).
- Assistant answers preserve arbitrary text (`blocks.ts:3582-3585`,
  `answer-body.ts:274-287`, `ui/answer-markdown.ts:158-160`). That path requires
  actual response text and has no 45s emoji timer. Mono CSS includes platform
  emoji fallback (`styles/themes/graphite.css:60-61`).

Therefore title/terminal/assistant input at the crash moment is needed to name
which glyph fired in the user's session. Timing alone cannot identify it.

## 4. Fix options and recommendation

**Recommend bundling jammy's security-current FreeType 2.11.1+dfsg-1ubuntu0.4**
(same experimental COLRv1 ABI as build-time .3), beside libwebkit in
`AppDir/usr/lib/libfreetype.so.6`. Copy the regular file and ship its licence.
Do not freeze the superseded .3 just to match a patch version. Record the exact
package version/hash in the build provenance and fail packaging on a wrong ABI.
`libwebkit`'s own `$ORIGIN` RUNPATH finds it; assert the actual loader resolution,
including the WebProcess, rather than relying on copy intent. External
`LD_LIBRARY_PATH` / preloads and earlier loading by another dependency must be
considered; bundle-first AppRun loader policy and process maps are the check.

Cost: a small distro shared library and notices, a packaging assertion, loader
proof and a renderer gate. No WebKit rebuild. Risk: the process has one loaded
FreeType SONAME, so host fontconfig/HarfBuzz as well as bundled GTK/Pango/Cairo
will use the old ABI generation. Check `ldd -r` and their imported FT symbols
on supported modern hosts. The independent installer PR does exactly this.
Fontconfig still reads **host fonts.conf and host fonts**, which is intentional;
repairing the ABI need not remove user's colour emoji.

Downloaded candidate:
https://archive.ubuntu.com/ubuntu/pool/main/f/freetype/libfreetype6_2.11.1+dfsg-1ubuntu0.4_amd64.deb

Its control metadata: `libbrotli1 >=0.6.0`, `libc6 >=2.33`,
`libpng16-16 >=1.6.2-1`, `zlib1g >=1:1.1.4`.
`readelf -d` exits 0 and names `libpng16.so.16`, `libz.so.1`,
`libbrotlidec.so.1`, `libc.so.6`. It does **not** depend on fontconfig or HarfBuzz
(the dependency points the other way). The existing image already contains
libpng16, libbrotlidec and libbrotlicommon; it excludes libz, so that still comes
from the host. No need to bundle unrelated fontconfig/HarfBuzz just to satisfy
FreeType's own closure. Exact host compatibility still needs execution.

Other options:

1. **New WebKit prebuilt:** Ubuntu noble's current 2.52.6 buildinfo uses
   FreeType 2.13.2 and libc6 2.39, GLib 2.80 and ICU74:
   https://launchpad.net/~ubuntu-security-proposed/+archive/ubuntu/ppa/+build/33529899/+files/webkit2gtk_2.52.6-0ubuntu0.24.04.1_amd64.buildinfo
   The present floor is jammy glibc2.35 (`release-build.yml:894-899`). Moving to
   noble changes the floor/dependency generation; not a drop-in fix. The
   buildinfo establishes build environment, **not by itself the ELF minimum
   GLIBC symbol version**; I did not inspect noble's ELF. A blanket claim that
   every symbol requires 2.39 would overstate the evidence. A source rebuild
   with the fork guard keeps the old floor but buys substantial build/security
   ownership (ADR-0035 rejects that standing obligation).
2. **Drop `_GLIBCXX_ASSERTIONS`: reject.** It requires rebuilding the prebuilt,
   does not repair any struct layout or out-of-bounds access, and changes a
   deterministic abort into undefined behaviour: out-of-range reads/writes,
   stack/heap corruption or later crash. Setting nocx compiler flags cannot
   remove checks already compiled into a distro .so.
3. **Fontconfig exclusion:** small packaging stopgap, but hiding only a named
   Noto family misses other COLRv1 fonts and @font-face fonts can bypass system
   selection. Broadly denying colour fonts costs colour emoji throughout nocx.
   It leaves the incompatible ABI armed for another caller. Not preferred.
4. **App-side replacement/filter:** removing a known emoji from one surface is
   cheap but cannot cover user terminal output, titles, assistant text, pasted
   Unicode or downloaded fonts. PNG pets already avoid this path. No global
   app-side input filtering is justified as a repair.

No concrete upstream WebKit/Ubuntu fix record was confirmed that makes the
bundle unnecessary. The cited rust-skia guard is the exact candidate. “Wait for
upstream” currently has no confirmed release/date and does not stop this P0.

## 5. Probe and CI plan — NOT EXECUTED

Untracked probe files in this worktree:

- `diagnosis-colrv1-probe.py`: ctypes GTK/WebKit harness. Loads GTK/WebKit/GLib
  and Cairo **from the extracted released AppImage**, not Playwright's engine.
  Uses that image's WebProcess helpers through the patched relative path;
  caller must chdir to AppDir before launching and provide library path.
  Creates a window and WebView; loads a data-URL `@font-face`, draws
  U+26A0+FE0F, U+1F308 and U+1F600 in DOM and canvas, awaits fonts and two
  animation frames, then requests WebKit's snapshot and writes PNG.
  Fails on `web-process-terminated`, timeout or failed snapshot.
- `diagnosis-noto-colrv1.ttf`: downloaded Google Fonts fixture
  `googlefonts/color-fonts/fonts/noto-glyf_colr_1.ttf`; pin commit
  `503f3d4950a60b30bd9c646d1b64a9d1237eafbb`, SHA256
  `fc1c3b581d01fcaed3283b9a12a04f784db0a22710e1c196dfb3a52696063fdc`.

**Neither script nor fixture has been validated/executed in a renderer.** The
current fixture is explicitly loaded @font-face, NOT installed as fontconfig
fallback. This isolates the ABI path but does not yet exercise actual nocx
fallback. The glyph COLRv1 gradient paint graph must be asserted with fontTools
before calling this a valid gradient-stop test. These are remaining steps,
not passing checks. The harness uses the shipped engine/helpers but not nocx's
whole window; the final bead check should additionally drive the real AppImage
window via a disposable shell title/output with that fontconfig fallback.

Expected probe output (plan, not captured evidence):

```
COLRV1_START webkit=2.50.4
LOADED_FREETYPE /path/.../libfreetype.so.6
COLRV1_PAINT_ACK canvas fillText + two animation frames; taking shipped renderer snapshot
COLRV1_SNAPSHOT status=0
```

The unrepaired artifact must instead show the exact ColorStop assertion and
nonzero exit / `WEB_PROCESS_TERMINATED`. A generic timeout is not accepted as
proof of the reported bug. A screenshot must visibly contain the chosen glyph
rather than tofu, and renderer process maps must identify the font/FT pairing.

CI placement: extend `scripts/appimage/verify-in-container.sh` / its foreign-host
runner, invoked by `release-build.yml:1120-1121` before artifact upload. Existing
gate has no font installed and succeeds as soon as a session opens, so it cannot
cover this bug. Use a pinned modern FreeType host (>=2.13), Xvfb, disposable
HOME/cache/fontconfig dir, pinned colour font with a verified gradient glyph,
and no host WebKit. Install fixture as fallback, print `fc-match` file/family,
`FT_Library_Version`, WebKit version/build ID, AppImage SHA256, loader resolved
path and glyph code points. Start actual AppImage, paint fixture in tab title
and terminal, then request a screenshot/paint ACK and demonstrate a subsequent
UI response. Capture WebProcess maps. Reject any outside-image FreeType binding
in the repaired case. Run exact same fixture against unrepaired 0.8.0 and show
this specific assertion. A mutation deleting bundled FT should turn the repaired
gate red. All evidence belongs in reporter output, not a silent liveness timer.

## Commands and runner outcome

Read AGENTS.md, docs/architecture.md (all invariants), docs/agents/backlog.md,
and diagnose-bug skill. `br show nocx-2inid`, `br show nocx-azxe.7`,
`git status --short --branch`, release queries, AppImage download/extractions,
SHA256, readelf/nm/objdump commands all exit 0. HTTP source/buildinfo fetches
returned 200. `readelf -p .comment` exits 0 but says section absent; no compiler
claim comes from it. `dpkg-deb -I` exited 127 (tool absent); metadata was read
by parsing ar and decompressing tar with `zstd` (both decompressions exit 0).
`nocx-sky-test --help` exits 2 by design (usage).

First probe environment command:

```
nocx-sky-test --e2e /home/dev/.herdr/worktrees/nocx/w-2inid \
 'command -v Xvfb; pkg-config --modversion freetype2; cat /etc/os-release; command -v fc-match; id'
```

Wrapper exits **3**, Sky launch exits **1**, with insufficient CPU/memory before
any NOCX_RUN_EXIT marker. **No product command ran.** Retried once with
`NOCX_SKY_CPUS=2 NOCX_SKY_MEMORY=4`; launch remained INIT. At coordinator's
instruction killed launcher (exit **-15**) and ran
`nocx-sky-test --down /home/dev/.herdr/worktrees/nocx/w-2inid` (exit **0**,
cluster terminated). No host tests ran. This is capacity, not product red.
The next run will use a new cluster or fresh pod temp directory to avoid the
remote stale-workdir issue reported separately as nocx-8e5rq.

## Remaining work before fix acceptance

1. Execute/validate the gradient fixture and shipped-renderer probe on a fresh
   pod when capacity permits; record the exact red assertion.
2. Make the approved AppImage assembly change, preserving updated notices and
   package provenance, with a bundle-first loader check.
3. Show red/green with **same** font/glyph and only FreeType pairing changed.
4. Add the artifact gate to nightly/release build; prove deletion mutation red.
5. Verify modern host fontconfig/HarfBuzz relocation compatibility and inspect
   screenshot. No bead closure, product-fix commit or success claim is due yet.

## Implementation follow-up

The original diagnosis probe/font were copied to
`/tmp/nocx-2inid-rehearsal-sfbfkugr/` before removal from the working tree.
They are historical inputs, not the shipped check. The maintained probe is
`scripts/appimage/colrv1-probe.py`; its pinned fixture is downloaded by
`scripts/appimage/colrv1-fixture.py`. See `implementation-2inid.md` for measured
red/green and remaining limits. The diagnosis above remains the pre-fix record.
