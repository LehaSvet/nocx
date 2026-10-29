# AppImage COLRv1 release check

`verify-appimage.sh <AppImage>` extends the existing Arch/Xvfb release gate.
Do not run `verify-in-container.sh` on a workstation. It installs packages and
mutates its extracted AppImage. The normal gate runs the repaired artifact
first, then removes FreeType and each helper/path repair in fresh containers.
An optional list after the artifact selects mutations for diagnosis.
`verify-appimage.sh original-v0.8.0.AppImage unbundled-baseline` asserts that
no FreeType ELF exists anywhere in the original artifact, then applies the
same fixture/probe and exact assertion requirement as `no-freetype`. It skips
repaired-package provenance and is deliberately not a default mutation.

The `none` case checks the exact FreeType package provenance shipped in the
image, modern-host relocations (`ldd -r`), and main/WebKitWebProcess memory
maps. Every repaired process must map the image's `usr/lib/libfreetype.so.6`.
Host FreeType must be >=2.13. The host has no GTK or WebKit package.

`colrv1-fixture.py` downloads, rather than vendors, the following test font:

- `googlefonts/color-fonts` commit
  `503f3d4950a60b30bd9c646d1b64a9d1237eafbb`,
  `fonts/noto-glyf_colr_1.ttf`.
- SHA256 `fc1c3b581d01fcaed3283b9a12a04f784db0a22710e1c196dfb3a52696063fdc`.
- The repository's Apache-2.0 LICENSE is downloaded and hash checked.
- Its Noto source submodule is pinned at
  `150e8ba1f68036bff1020c17e3f6b09530fd9d8a`. That source's `fonts/LICENSE`
  (SIL OFL 1.1) is also downloaded and hash checked. Both notices stay beside
  the downloaded fixture. No modified font is distributed.

FontTools traverses each candidate glyph's complete COLRv1 paint graph.
It reports each of U+26A0+FE0F, U+1F308 and U+1F600, including solid-only
graphs, and requires a gradient with at least two ColorStops in the set. It
prints code points, glyph names, font family, path, hash and each gradient
format/stop count. A COLRv1 table alone
or a cmap entry is not enough.

`colrv1-probe.py` loads only the image's GTK/WebKit libraries and helpers.
It first reports real loader maps while the renderer is alive, then loads the
font with `@font-face`, paints all three glyphs in DOM and canvas, waits for two
animation frames, and takes a WebKit/Cairo snapshot. DOM glyph bounds and canvas
bounds come from their own `getBoundingClientRect`. DOM glyphs, canvas rainbow
and canvas grinning face must contain colour pixels. U+26A0 is allowed text
presentation on canvas only; its full FE0F DOM sequence still requires colour.
The grinning-face canvas graph contains a verified radial gradient, so a solid
rainbow alone cannot satisfy the canvas check. A second snapshot replaces DOM
glyphs with monospace U+10FFFF missing-glyph controls. Each colour glyph must
exceed its control's chromatic pixel count, hue buckets and mean saturation.
Cairo pixels are inspected directly, without host image-library dependencies. A new JS message after the snapshot
proves the renderer still responds. The `no-freetype` mutation must instead
produce a nonzero result AND the exact `colrv1_configure_skpaint` / `ColorStop`
vector `__n < this->size()` assertion. Generic crashes or timeouts fail the
gate rather than passing the negative control.

The same font is installed in isolated fontconfig fallback for the actual
AppImage. The gate checks `fc-match` for both sans and mono. The actual PTY
shell prints the glyph in an OSC title and output. The gate captures the
actual window and sends a subsequent command through X11 keyboard events.
The command must produce a response file, and the renderer must still map the
bundled FreeType afterwards. This checks survival and input response, not
semantic screenshot recognition of the actual app's tab and grid: inspect the
two AppImage PNGs to confirm those glyphs visibly appear. The standalone
shipped-engine DOM/canvas snapshots do have automated colour-pixel checks.

Evidence stays in `<artifact directory>/appimage-verification/<mutation>/`.
Set `NOCX_APPIMAGE_VERIFY_EVIDENCE` to choose another directory. Logs print
in the gate output. The font itself is not copied to retained evidence;
`fixture.json` identifies the exact bytes and selected glyph.

## Optional host comparison

The default foreign host stays Arch. `NOCX_APPIMAGE_VERIFY_IMAGE=ubuntu:24.04`
uses the same harness with apt dependencies and explicit no-host-GTK/WebKit
assertions. Printouts identify the actual FreeType version. A successful paint
is not accepted as a negative-control reproduction.

## Diagnostic trial limits

During diagnosis an optional richer fixture trial selected a 13-stop linear
gradient glyph, a 101-layer glyph and shaped ZWJ sequences. The original image
also produced the exact ColorStop assertion for those static scenarios. A
separate variable-font trial reached fixture preparation, but its default
coordinate snapshot did not prove colour rendering; the maximum coordinate
scenario did not run. Variable rendering remains unproven. The experimental
helper and wiring were removed rather than retained as an unvalidated release
check. The final bounded gate uses the three verified default glyphs only.
