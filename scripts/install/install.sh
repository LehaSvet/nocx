#!/bin/sh
# Install nocx for the current Linux user. Usage: curl -fsSL https://nocx.dev/install.sh | sh
set -eu

BASE=https://github.com/shady2k/nocx/releases/latest/download
PUBKEY_DER_B64=MCowBQYDK2VwAyEAmApMKKml14qq2l/B/VGAE0BBWf7NEKFuId7jpZzo+eE=

fail() { printf 'nocx installer: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Linux) ;;
  Darwin) fail 'macOS is installed with Homebrew: brew install --cask shady2k/nocx/nocx' ;;
  *) fail "unsupported operating system: $(uname -s); this installer supports x86_64 Linux only" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) ;;
  *) fail "unsupported architecture: $(uname -m); this installer supports x86_64 Linux only" ;;
esac
command -v curl >/dev/null 2>&1 || fail 'curl is required; install curl and run this command again'
command -v openssl >/dev/null 2>&1 || fail 'OpenSSL 3 or newer is required for signed-manifest verification. Install OpenSSL 3 (for example: sudo apt install openssl) and run this command again.'
openssl_version=$(openssl version 2>/dev/null) || fail 'could not run openssl; install OpenSSL 3 or newer and retry'
openssl_major=$(printf '%s\n' "$openssl_version" | sed -n 's/^OpenSSL \([0-9][0-9]*\)\..*/\1/p')
case "$openssl_major" in
  ''|*[!0-9]*) fail 'could not determine OpenSSL version; OpenSSL 3 or newer is required' ;;
esac
[ "$openssl_major" -ge 3 ] || fail "OpenSSL 3 or newer is required (found: $openssl_version). Install OpenSSL 3 (for example: sudo apt install openssl) and run this command again."
command -v awk >/dev/null 2>&1 || fail 'POSIX awk is required; install awk and retry'
command -v sed >/dev/null 2>&1 || fail 'POSIX sed is required; install sed and retry'

umask 077
tmp=$(mktemp -d "${TMPDIR:-/tmp}/nocx-install.XXXXXXXX") || fail 'cannot create a temporary directory'
install_tmp=
desktop_tmp=
icon_tmp=
backup_app=
backup_desktop=
backup_icon=
committed=0
app_replaced=0
desktop_replaced=0
icon_replaced=0
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT HUP INT TERM

curl --fail --silent --show-error --location "$BASE/manifest.json" -o "$tmp/manifest.json" || fail 'could not download release manifest'
curl --fail --silent --show-error --location "$BASE/manifest.json.sig" -o "$tmp/manifest.json.sig" || fail 'could not download release manifest signature'
# Verify the exact downloaded bytes before parsing any JSON.
printf '%s' "$PUBKEY_DER_B64" | openssl base64 -d -A > "$tmp/release-public-key.der" || fail 'could not prepare the built-in release public key'
openssl pkey -pubin -inform DER -in "$tmp/release-public-key.der" -out "$tmp/release-public-key.pem" >/dev/null 2>&1 || fail 'could not load the built-in release public key'
# The release signature is base64-encoded detached Ed25519 over raw manifest bytes.
openssl base64 -d -A -in "$tmp/manifest.json.sig" -out "$tmp/manifest.sig" 2>/dev/null || fail 'release manifest signature is not valid base64'
openssl pkeyutl -verify -pubin -inkey "$tmp/release-public-key.pem" -rawin -in "$tmp/manifest.json" -sigfile "$tmp/manifest.sig" >/dev/null 2>&1 || fail 'release manifest signature verification failed; nothing was installed'

# Parse only after signature verification. Manifest objects are emitted by the release
# workflow. The scanner tracks JSON object/array nesting and extracts one flat artifact
# object without depending on whitespace or field order. This deliberately accepts only
# the JSON string/number forms used by the release manifest and rejects escaped values.
artifact=$(awk '
function trim(s) { sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s); return s }
function unquote(s) { s=trim(s); if (s !~ /^"[^"\\]*"$/) return ""; sub(/^"/, "", s); sub(/"$/, "", s); return s }
{
  text = text $0 "\n"
}
END {
  s=text; start=0; finish=0; depth=0; quoted=0; escaped=0
  # Locate artifacts only as a top-level member of the signed manifest.
  for (i=1; i<=length(s); i++) {
    c=substr(s,i,1)
    if (!quoted && depth == 1 && c == "\"" && substr(s,i) ~ /^"artifacts"[[:space:]]*:/) {
      tail=substr(s,i); if (match(tail, /:[[:space:]]*\[/)) start=i+RSTART+RLENGTH-2
      break
    }
    if (quoted) { if (escaped) escaped=0; else if (c == "\\") escaped=1; else if (c == "\"") quoted=0; continue }
    if (c == "\"") { quoted=1; continue }
    if (c == "{") depth++
    else if (c == "}") depth--
  }
  if (start == 0) exit 2
  quoted=0; escaped=0; bracket=1; object_depth=0; nested=0; obj=""; count=0
  for (i=start+1; i<=length(s); i++) {
    c=substr(s,i,1)
    if (quoted) { if (escaped) escaped=0; else if (c == "\\") escaped=1; else if (c == "\"") quoted=0; if (object_depth) obj=obj c; continue }
    if (c == "\"") { quoted=1; if (object_depth) obj=obj c; continue }
    if (c == "{") { object_depth++; if (object_depth == 1) { obj="{"; nested=0 } else if (object_depth) { nested=1; obj=obj c } continue }
    if (c == "}") {
      if (!object_depth) exit 2
      obj=obj c; object_depth--
      if (!object_depth) {
        if (obj ~ /"os"[[:space:]]*:[[:space:]]*"linux"/ && obj ~ /"arch"[[:space:]]*:[[:space:]]*"amd64"/ && obj ~ /"format"[[:space:]]*:[[:space:]]*"AppImage"/) {
          if (nested) exit 2
          count++; if (count > 1) exit 2
          if (obj ~ /"os"[^}]*"os"/ || obj ~ /"arch"[^}]*"arch"/ || obj ~ /"format"[^}]*"format"/ || obj ~ /"url"[^}]*"url"/ || obj ~ /"sha256"[^}]*"sha256"/ || obj ~ /"size"[^}]*"size"/) exit 2
          url=""; sha=""; size=""
          if (match(obj, /"url"[[:space:]]*:[[:space:]]*"[^"\\]*"/)) url=substr(obj,RSTART,RLENGTH)
          if (match(obj, /"sha256"[[:space:]]*:[[:space:]]*"[^"\\]*"/)) sha=substr(obj,RSTART,RLENGTH)
          if (match(obj, /"size"[[:space:]]*:[[:space:]]*[0-9]+/)) size=substr(obj,RSTART,RLENGTH)
          url=unquote(substr(url,index(url,":")+1)); sha=unquote(substr(sha,index(sha,":")+1))
          sub(/^[^:]*:[[:space:]]*/, "", size)
          if (url !~ /^https:\/\/github.com\/shady2k\/nocx\/releases\/download\//) exit 2
          if (length(sha) != 64 || sha !~ /^[0123456789abcdefABCDEF]+$/) exit 2
          if (size !~ /^[0-9]+$/ || size == "0") exit 2
          print url "\t" sha "\t" size
        }
        obj=""
      }
      continue
    }
    if (object_depth) obj=obj c
    if (!object_depth) {
      if (c == "[") bracket++
      else if (c == "]") { bracket--; if (!bracket) { finish=1; break } }
    }
  }
  if (!finish || object_depth || count != 1) exit 2
}
' "$tmp/manifest.json") || fail 'verified manifest has no single valid linux/amd64 AppImage artifact'
IFS="$(printf '\t')" read -r artifact_url artifact_sha artifact_size <<EOF
$artifact
EOF
[ -n "$artifact_url" ] && [ -n "$artifact_sha" ] && [ -n "$artifact_size" ] || fail 'verified manifest artifact fields are incomplete'

curl --fail --silent --show-error --location "$artifact_url" -o "$tmp/nocx.AppImage" || fail 'could not download the AppImage'
curl --fail --silent --show-error --location https://raw.githubusercontent.com/shady2k/nocx/bf175aae5d4639bdc77a145bf6b7b5e6c47ec182/site/assets/icon-256.png -o "$tmp/nocx.png" || fail 'could not download nocx icon'
actual_size=$(wc -c < "$tmp/nocx.AppImage" | tr -d ' ')
[ "$actual_size" = "$artifact_size" ] || fail 'downloaded AppImage size does not match the signed manifest'
actual_sha=$(openssl dgst -sha256 "$tmp/nocx.AppImage" | sed 's/^.*= //')
[ "$(printf '%s' "$actual_sha" | tr 'A-F' 'a-f')" = "$(printf '%s' "$artifact_sha" | tr 'A-F' 'a-f')" ] || fail 'downloaded AppImage SHA-256 does not match the signed manifest'

home=${HOME:?HOME must be set}
bin_dir=$home/.local/bin
applications_dir=$home/.local/share/applications
icons_dir=$home/.local/share/icons/hicolor/256x256/apps
mkdir -p "$bin_dir" "$applications_dir" "$icons_dir" || fail 'cannot create user install directories'
# Prepare every file before the first persistent replacement. The AppImage stays in
# the target directory so its final rename is atomic and remains user-owned.
install_tmp="$bin_dir/.nocx.install.$$"
desktop_tmp="$applications_dir/.nocx.desktop.$$"
icon_tmp="$icons_dir/.nocx.png.$$"
backup_app="$bin_dir/.nocx.backup.$$"
backup_desktop="$applications_dir/.nocx.desktop.backup.$$"
backup_icon="$icons_dir/.nocx.png.backup.$$"
cleanup_install() {
  [ -z "$install_tmp" ] || rm -f "$install_tmp"
  [ -z "$desktop_tmp" ] || rm -f "$desktop_tmp"
  [ -z "$icon_tmp" ] || rm -f "$icon_tmp"
  [ -z "$backup_app" ] || rm -f "$backup_app"
  [ -z "$backup_desktop" ] || rm -f "$backup_desktop"
  [ -z "$backup_icon" ] || rm -f "$backup_icon"
}
rollback_install() {
  [ "$committed" -eq 0 ] || return 0
  if [ -n "$backup_app" ] && [ -f "$backup_app" ]; then mv -f "$backup_app" "$bin_dir/nocx"; elif [ "$app_replaced" -eq 1 ]; then rm -f "$bin_dir/nocx"; fi
  if [ -n "$backup_desktop" ] && [ -f "$backup_desktop" ]; then mv -f "$backup_desktop" "$applications_dir/nocx.desktop"; elif [ "$desktop_replaced" -eq 1 ]; then rm -f "$applications_dir/nocx.desktop"; fi
  if [ -n "$backup_icon" ] && [ -f "$backup_icon" ]; then mv -f "$backup_icon" "$icons_dir/nocx.png"; elif [ "$icon_replaced" -eq 1 ]; then rm -f "$icons_dir/nocx.png"; fi
}
trap 'rollback_install; cleanup_install; cleanup' EXIT
trap 'rollback_install; cleanup_install; cleanup; exit 1' HUP INT TERM
cp "$tmp/nocx.AppImage" "$install_tmp" || fail 'cannot stage verified AppImage in ~/.local/bin'
chmod 755 "$install_tmp" || fail 'cannot make staged AppImage executable'
printf '[Desktop Entry]\nName=nocx\nExec=%s\nIcon=nocx\nType=Application\nCategories=Utility;TerminalEmulator;\nStartupWMClass=nocx\n' "$bin_dir/nocx" > "$desktop_tmp" || fail 'cannot stage desktop entry'
cp "$tmp/nocx.png" "$icon_tmp" || fail 'cannot stage nocx icon'
# Back up existing user files before replacement. If any rename fails, the exit
# trap restores them (or removes newly created files) before dropping staging data.
[ ! -e "$bin_dir/nocx" ] || cp -p "$bin_dir/nocx" "$backup_app" || fail 'cannot preserve existing AppImage before upgrade'
[ ! -e "$applications_dir/nocx.desktop" ] || cp -p "$applications_dir/nocx.desktop" "$backup_desktop" || fail 'cannot preserve existing desktop entry'
[ ! -e "$icons_dir/nocx.png" ] || cp -p "$icons_dir/nocx.png" "$backup_icon" || fail 'cannot preserve existing icon'
# Replace stable user-owned AppImage path used by updater, then metadata.
mv -f "$install_tmp" "$bin_dir/nocx" || fail 'cannot install AppImage'
app_replaced=1
install_tmp=
mv -f "$desktop_tmp" "$applications_dir/nocx.desktop" || fail 'cannot install desktop entry'
desktop_replaced=1
desktop_tmp=
mv -f "$icon_tmp" "$icons_dir/nocx.png" || fail 'cannot install nocx icon'
icon_replaced=1
icon_tmp=
committed=1
printf 'Installed nocx %s. Start it with: %s\n' "$(printf '%s' "$artifact_url" | sed 's|.*/||; s/^nocx-//; s/-linux-amd64.AppImage$//')" "$bin_dir/nocx"
