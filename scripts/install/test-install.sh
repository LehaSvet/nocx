#!/bin/sh
# End-to-end installer tests using local release fixtures and a generated test key.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
INSTALLER=$ROOT/scripts/install/install.sh
TMP=$(mktemp -d "${TMPDIR:-/tmp}/nocx-installer-test.XXXXXXXX")
trap 'rm -rf "$TMP"' EXIT HUP INT TERM
TEST_INSTALLER=$TMP/test-install.sh
PAGES_INSTALLER=$TMP/pages/site/install.sh
mkdir -p "$TMP/bin" "$TMP/release" "$TMP/home" "$(dirname "$PAGES_INSTALLER")"

openssl genpkey -algorithm ED25519 -out "$TMP/private.pem" >/dev/null 2>&1
openssl pkey -in "$TMP/private.pem" -pubout -out "$TMP/public.pem" >/dev/null 2>&1
cp "$INSTALLER" "$PAGES_INSTALLER"
cmp "$INSTALLER" "$PAGES_INSTALLER"
test -x "$PAGES_INSTALLER"
pub_b64=$(openssl pkey -in "$TMP/private.pem" -pubout -outform DER | openssl base64 -A)
cp "$INSTALLER" "$TEST_INSTALLER"
sed -i "s|^BASE=.*|BASE=https://releases.test/latest|; s|^PUBKEY_DER_B64=.*|PUBKEY_DER_B64=$pub_b64|; s|https://raw.githubusercontent.com/shady2k/nocx/bf175aae5d4639bdc77a145bf6b7b5e6c47ec182/site/assets/icon-256.png|https://icons.test/icon.png|g" "$TEST_INSTALLER"
APP='fake AppImage payload bytes'
printf '%s' "$APP" > "$TMP/release/app.AppImage"
size=$(wc -c < "$TMP/release/app.AppImage" | tr -d ' ')
sha=$(sha256sum "$TMP/release/app.AppImage" | awk '{print $1}')
make_manifest() {
  style=$1
  if [ "$style" = release ]; then
    cp "$ROOT/scripts/install/testdata/manifest-v0.4.0.json" "$TMP/release/manifest.json"
  elif [ -n "${MANIFEST_MODE:-}" ]; then
    case "$MANIFEST_MODE" in
      duplicate) printf '{"artifacts":[{"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/x","sha256":"%s","size":%s},{"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/y","sha256":"%s","size":%s}]}' "$sha" "$size" "$sha" "$size" > "$TMP/release/manifest.json" ;;
      invalid-sha) printf '{"artifacts":[{"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/x","sha256":"bad","size":%s}]}' "$size" > "$TMP/release/manifest.json" ;;
      bad-url) printf '{"artifacts":[{"os":"linux","arch":"amd64","format":"AppImage","url":"https://evil.example/x","sha256":"%s","size":%s}]}' "$sha" "$size" > "$TMP/release/manifest.json" ;;
      malformed) printf '{"artifacts":[{"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/x","sha256":"%s","size":%s}' "$sha" "$size" > "$TMP/release/manifest.json" ;;
      nested-decoy) printf '{"artifacts":[{"metadata":{"os":"linux","arch":"amd64","format":"AppImage"},"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/x","sha256":"%s","size":%s}] }' "$sha" "$size" > "$TMP/release/manifest.json" ;;
      helper-decoy) printf '{"artifacts":[],"helpers":[{"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/x","sha256":"%s","size":%s}]}' "$sha" "$size" > "$TMP/release/manifest.json" ;;
      no-match) printf '{"artifacts":[{"os":"linux","arch":"arm64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/x","sha256":"%s","size":%s}]}' "$sha" "$size" > "$TMP/release/manifest.json" ;;
      *) exit 9 ;;
    esac
  elif [ "$style" = compact ]; then
    printf '{"version":"0.3.1","released":"2026-10-16T00:00:00Z","notesUrl":"https://example.test","artifacts":[{"os":"linux","arch":"amd64","format":"AppImage","url":"https://github.com/shady2k/nocx/releases/download/v0.3.1/nocx.AppImage","sha256":"%s","size":%s}],"helpers":[]}' "$sha" "$size" > "$TMP/release/manifest.json"
  else
    jq -n --arg sha "$sha" --argjson size "$size" \
      --arg url "https://github.com/shady2k/nocx/releases/download/v0.3.1/nocx.AppImage" \
      '{version:"0.3.1", released:"2026-10-16T00:00:00Z", notesUrl:"https://example.test", artifacts:[{os:"linux",arch:"amd64",format:"AppImage",url:$url,sha256:$sha,size:$size}], helpers:[]}' > "$TMP/release/manifest.json"
  fi
  openssl pkeyutl -sign -inkey "$TMP/private.pem" -rawin -in "$TMP/release/manifest.json" -out "$TMP/release/manifest.sig"
  openssl base64 -A -in "$TMP/release/manifest.sig" > "$TMP/release/manifest.json.sig"
}
cat > "$TMP/bin/curl" <<'CURL'
#!/bin/sh
set -eu
out=
url=
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
case "${CURL_FAIL:-}:$url" in
  app:*/nocx.AppImage|icon:*/icon.png) exit 22 ;;
  *:*/manifest.json) src=$TEST_RELEASE/manifest.json ;;
  *:*/manifest.json.sig) src=$TEST_RELEASE/manifest.json.sig ;;
  *:*/nocx.AppImage|*/nocx-*-linux-amd64.AppImage) src=$TEST_RELEASE/app.AppImage ;;
  *:*/icon.png) src=$TEST_RELEASE/icon.png ;;
  *) echo "unexpected curl URL: $url" >&2; exit 22 ;;
esac
[ -f "$src" ] || exit 22
cp "$src" "$out"
CURL
chmod +x "$TMP/bin/curl"
printf 'pngfixture' > "$TMP/release/icon.png"
cat > "$TMP/bin/mv" <<'MV'
#!/bin/sh
if [ -z "${MV_COUNT_FILE:-}" ]; then exec /run/current-system/sw/bin/mv "$@"; fi
count_file=$MV_COUNT_FILE
count=0
[ ! -f "$count_file" ] || count=$(cat "$count_file")
count=$((count+1))
printf '%s' "$count" > "$count_file"
if [ "${MV_FAIL_AT:-0}" = "$count" ]; then exit 1; fi
exec /run/current-system/sw/bin/mv "$@"
MV
chmod +x "$TMP/bin/mv"
REAL_MV=$(command -v mv)
cat > "$TMP/bin/uname" <<'UNAME'

#!/bin/sh
case "$1" in -s) echo "${TEST_UNAME_OS:-Linux}";; -m) echo "${TEST_UNAME_ARCH:-x86_64}";; *) exit 1;; esac
UNAME
chmod +x "$TMP/bin/uname"

run_ok() {
  style=$1; make_manifest "$style"
  if HOME="$TMP/home" PATH="$TMP/bin:$PATH" TEST_RELEASE="$TMP/release" \
    sh "$TEST_INSTALLER" > "$TMP/stdout" 2> "$TMP/stderr"; then :; else cat "$TMP/stderr" >&2; exit 1; fi
  test -x "$TMP/home/.local/bin/nocx"
  cmp "$TMP/release/app.AppImage" "$TMP/home/.local/bin/nocx"
  test -f "$TMP/home/.local/share/applications/nocx.desktop"
  test -f "$TMP/home/.local/share/icons/hicolor/256x256/apps/nocx.png"
}
run_ok compact
run_ok indented
# The checked-in file is the actual jq-generated release manifest. Its signed
# bytes are test-signed; parsing must select exactly its linux artifact. Payload
# size mismatch is expected because the integration fixture is deliberately tiny.
cp "$TMP/home/.local/bin/nocx" "$TMP/release/expected-old"
make_manifest release
if HOME="$TMP/home" PATH="$TMP/bin:$PATH" TEST_RELEASE="$TMP/release" sh "$TEST_INSTALLER" > "$TMP/stdout" 2> "$TMP/stderr"; then
  echo 'real release manifest with fake payload unexpectedly installed' >&2; exit 1
fi
if ! grep -q 'size does not match' "$TMP/stderr"; then cat "$TMP/stderr" >&2; exit 1; fi
cmp "$TMP/release/expected-old" "$TMP/home/.local/bin/nocx"
# Upgrade replaces the same user-owned path and remains executable.
printf 'second verified payload' > "$TMP/release/app.AppImage"
size=$(wc -c < "$TMP/release/app.AppImage" | tr -d ' ')
sha=$(sha256sum "$TMP/release/app.AppImage" | awk '{print $1}')
run_ok compact
cp "$TMP/home/.local/bin/nocx" "$TMP/old-app"
cp "$TMP/home/.local/share/applications/nocx.desktop" "$TMP/old-desktop"
cp "$TMP/home/.local/share/icons/hicolor/256x256/apps/nocx.png" "$TMP/old-icon"
printf 'third payload version' > "$TMP/release/app.AppImage"
size=$(wc -c < "$TMP/release/app.AppImage" | tr -d ' ')
sha=$(sha256sum "$TMP/release/app.AppImage" | awk '{print $1}')
make_manifest compact
for fail_at in 2 3; do
  rm -f "$TMP/MV_COUNT"
  if HOME="$TMP/home" PATH="$TMP/bin:$PATH" TEST_RELEASE="$TMP/release" REAL_MV="$REAL_MV" MV_COUNT_FILE="$TMP/MV_COUNT" MV_FAIL_AT="$fail_at" sh "$TEST_INSTALLER" > "$TMP/stdout" 2> "$TMP/stderr"; then
    echo "injected rename failure $fail_at unexpectedly succeeded" >&2; exit 1
  fi
  cmp "$TMP/old-app" "$TMP/home/.local/bin/nocx"
  cmp "$TMP/old-desktop" "$TMP/home/.local/share/applications/nocx.desktop"
  cmp "$TMP/old-icon" "$TMP/home/.local/share/icons/hicolor/256x256/apps/nocx.png"
  test -z "$(find "$TMP/home" -name '.nocx.*' -o -name '.nocx.install.*')"
done

assert_refused_without_install() {
  label=$1
  shift
  rm -f "$TMP/home/.local/bin/nocx" "$TMP/home/.local/share/applications/nocx.desktop" "$TMP/home/.local/share/icons/hicolor/256x256/apps/nocx.png"
  if HOME="$TMP/home" PATH="$TMP/bin:$PATH" TEST_RELEASE="$TMP/release" \
      "$@" sh "$TEST_INSTALLER" > "$TMP/stdout" 2> "$TMP/stderr"; then
    echo "$label unexpectedly succeeded" >&2; exit 1
  fi
  test ! -e "$TMP/home/.local/bin/nocx"
  test ! -e "$TMP/home/.local/share/applications/nocx.desktop"
  test ! -e "$TMP/home/.local/share/icons/hicolor/256x256/apps/nocx.png"
}
# Authenticated but structurally invalid manifests must fail closed.
for mode in duplicate invalid-sha bad-url no-match malformed helper-decoy nested-decoy; do
  MANIFEST_MODE=$mode make_manifest compact
  openssl pkeyutl -sign -inkey "$TMP/private.pem" -rawin -in "$TMP/release/manifest.json" -out "$TMP/release/manifest.sig"
  openssl base64 -A -in "$TMP/release/manifest.sig" > "$TMP/release/manifest.json.sig"
  assert_refused_without_install "manifest $mode"
done
unset MANIFEST_MODE
# Tampered manifest: signature remains for the original bytes. A failed re-run must preserve an existing install.
make_manifest compact
printf ' ' >> "$TMP/release/manifest.json"
assert_refused_without_install 'tampered manifest'
# A refused rerun must leave an existing user-owned AppImage untouched.
printf 'previous valid install' > "$TMP/home/.local/bin/nocx"
make_manifest compact
printf ' ' >> "$TMP/release/manifest.json"
if HOME="$TMP/home" PATH="$TMP/bin:$PATH" TEST_RELEASE="$TMP/release" \
    sh "$TEST_INSTALLER" > "$TMP/stdout" 2> "$TMP/stderr"; then
  echo 'tampered manifest rerun unexpectedly succeeded' >&2; exit 1
fi
printf 'previous valid install' > "$TMP/expected"
cmp "$TMP/expected" "$TMP/home/.local/bin/nocx"
rm -f "$TMP/home/.local/bin/nocx"

# Tampered artifact: signed manifest still describes original digest/size.
make_manifest compact
printf 'tampered' >> "$TMP/release/app.AppImage"
assert_refused_without_install 'tampered artifact'
# Network failure downloading payload/icon also leaves destination files absent.
make_manifest compact
assert_refused_without_install 'AppImage download failure' env CURL_FAIL=app
make_manifest compact
assert_refused_without_install 'icon download failure' env CURL_FAIL=icon
# Platform refusals happen before network access or writes.
for platform in Darwin: x86_64 Linux:aarch64; do
  TEST_UNAME_OS=${platform%:*}; TEST_UNAME_ARCH=${platform#*:}
  rm -f "$TMP/home/.local/bin/nocx"
  if HOME="$TMP/home" PATH="$TMP/bin:$PATH" TEST_UNAME_OS="$TEST_UNAME_OS" TEST_UNAME_ARCH="$TEST_UNAME_ARCH" sh "$TEST_INSTALLER" > "$TMP/stdout" 2> "$TMP/stderr"; then
    echo "unsupported platform $platform unexpectedly succeeded" >&2; exit 1
  fi
  test ! -e "$TMP/home/.local/bin/nocx"
  case "$TEST_UNAME_OS" in Darwin) grep -q 'brew install --cask shady2k/nocx/nocx' "$TMP/stderr";; esac
done
# Missing and too-old OpenSSL fail before any curl or install write.
cat > "$TMP/bin/openssl" <<'MISSING'
#!/bin/sh
case "$1" in version) echo 'OpenSSL 3.0.0'; exit 0;; esac
exit 127
MISSING
chmod +x "$TMP/bin/openssl"
# remove executable to ensure no installer write occurred
assert_refused_without_install 'broken verifier'
cat > "$TMP/bin/openssl" <<'OLD'
#!/bin/sh
echo 'OpenSSL 1.1.1'
OLD
chmod +x "$TMP/bin/openssl"
assert_refused_without_install 'old OpenSSL'
grep -q 'OpenSSL 3 or newer is required' "$TMP/stderr"
# Missing OpenSSL is checked by a PATH with all tools except the OpenSSL wrapper.
rm -f "$TMP/bin/openssl"
mkdir -p "$TMP/noopenssl"
for tool in sh uname sed awk curl mktemp rm printf tr wc cp chmod mv mkdir cat; do
  tool_path=$(command -v "$tool")
  ln -s "$tool_path" "$TMP/noopenssl/$tool"
done
assert_refused_without_install 'missing OpenSSL' env PATH="$TMP/noopenssl" TEST_RELEASE="$TMP/release"
grep -q 'OpenSSL 3 or newer is required' "$TMP/stderr"

echo 'installer tests passed'
