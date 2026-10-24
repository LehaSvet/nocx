# ADR-0103 — One desktop application identity is dev.nocx

- **Status:** Accepted
- **Date:** 2026-10-24
- **Supersedes:** No accepted ADR records `io.github.shady2k.nocx` or
  `com.shady2k.nocx`; this decision replaces the packaging values, not an
  invented prior decision. ADR-0052's separate coordinator identity stands.
- **Beads:** `nocx-1ebrs.51`, `nocx-llinl`

## Context

The owner chose `dev.nocx` as the application identifier on 2026-10-24.
Development identities use the existing `-dev` suffix: `dev.nocx-dev`.
This is greenfield. There is no migration of OS-owned state.

The release workflow passed `-productidentifier com.shady2k.nocx` to Wails v3
beta.9, then copied `build/assets/darwin/Info.plist` into the assembled app.
The committed `build/darwin/Info.plist` instead declared
`io.github.shady2k.nocx`. Neither it nor `Info.dev.plist` is read by this
workflow: `GenerateBuildAssets` extracts Wails' embedded templates into its
`-dir` destination. The committed templates were not the shipped identity. `nocx-n0z7` carried
the old template-based reasoning; `nocx-1ebrs.51` records the discrepancy with
the assembled release. Neither is an accepted ADR to edit or supersede.
The generator's copyright default and its identifier default are likewise
upstream defaults, not values read from these committed files.

Wails beta.9 constructs the Linux GtkApplication ID as
`org.wails.<sanitized name>` in both its GTK3 and GTK4 backends. There is no
application-ID option. `Linux.ProgramName` calls `g_set_prgname`; it does not
change the GtkApplication ID. A launcher filename guessed from only one of
these facts cannot prove the Wayland window's identity.

The question was how to align the running Linux application and its launcher
without assuming that the GtkApplication ID and Wayland `app_id` were the
same fact. A pinned dependency fork, a vendored dependency and a launcher-only
rename were considered. The first two add dependency maintenance; the last
cannot satisfy the owner's chosen identity if the runtime remains unchanged.
The open question was whether a supported pre-registration GLib call could
align the GtkApplication ID and the compositor's `app_id`, without a dependency
fork, vendored module or launcher-only identity compromise. Measurement settled
it: GLib changes the GtkApplication ID, and `Linux.ProgramName` independently
changes GTK3's Wayland `app_id`. Both are required.

## Decision

The macOS release explicitly passes `-productidentifier dev.nocx`. Keep this
flag: removing it would restore the generator's `com.wails.` default, not read
the committed plist. The committed `Info.plist` agrees with the release and
names its actual status. Delete unread `Info.dev.plist`. `make dev`, `go run`
and `make build` currently produce an unbundled executable, not a development
`.app`; `dev.nocx-dev` has no macOS bundle consumer today. This decision does
not invent a dev bundle target.

A release reads `CFBundleIdentifier` from the assembled
`build/bin/nocx.app/Contents/Info.plist` with PlistBuddy and refuses anything
except `dev.nocx`, including a missing or unreadable key. The flag assertion
lives beside the existing About identity assertions. Its regression check
executes the release shell check with a substituted plist reader and proves
that old, dev, empty and unreadable identifiers are refused. This is not a
claim that a Linux unit test inspected a shipped macOS bundle; the actual
assembled-bundle check runs during release assembly.

### Linux: two supported settings, no dependency change

Set `Linux.ProgramName` to the build's desktop identity and register the
identity service first. Wails beta.9 `App.Run` constructs the platform app,
starts services, and only then calls the platform `run`. The identity service
uses `g_application_get_default`, refuses a missing or already registered
application, calls `g_application_set_application_id`, and reads it back. The
setter is a [supported GLib API](https://docs.gtk.org/gio/method.Application.set_application_id.html)
required to run before registration. No Wails fork, patch or vendored module
is needed. Ordinary builds use the same path.

GTK3's Wayland backend uses `g_get_prgname()` for `xdg_toplevel.set_app_id`,
falling back to the GDK program class; it does not derive this value from the
GtkApplication ID in this path. This is why setting the GLib ID alone is not
enough. Keep the product name `nocx`; the program identity is separate from
its displayed name.

The actual GTK3 application was run under a Wayland compositor. A temporary
observer of public GApplication entry/startup calls read the application ID
before `g_application_run` and after registration without mutating it.
`WAYLAND_DEBUG` captured the `xdg_toplevel.set_app_id` request and subsequent
buffer commits. This measured windows and identity, not terminal functionality.

| Configuration                                                     | GtkApplication ID (before run / after registration) | `prgname`      | Compositor `app_id` |
| ----------------------------------------------------------------- | --------------------------------------------------- | -------------- | ------------------- |
| Unchanged control (identity service absent, original ProgramName) | `org.wails.nocx` / `org.wails.nocx`                 | `nocx`         | `nocx`              |
| GLib setter only                                                  | `dev.nocx` / `dev.nocx`                             | `nocx`         | `nocx`              |
| Both, release                                                     | `dev.nocx` / `dev.nocx`                             | `dev.nocx`     | `dev.nocx`          |
| Both, development                                                 | `dev.nocx-dev` / `dev.nocx-dev`                     | `dev.nocx-dev` | `dev.nocx-dev`      |

The unchanged control continued to start, demonstrating that unmodified Wails
still behaves as before rather than requiring a patched dependency. GTK4 has
the same pre-registration lifecycle in beta.9; its runtime identity was not
measured here. The shipped Linux GTK3 path was measured.

The AppImage desktop entry and user-installed desktop entry are
`dev.nocx.desktop`, matching the measured release `app_id`; the visible name,
executable and icon remain `nocx`. Installer fixture tests check the installed
basename and identity metadata. Development runtime identity is
`dev.nocx-dev`; no development AppImage/installer target is introduced.

## State and consent consequences

Do not promise that notification consent is the only reset. The table separates
product-owned state proven by the tree from OS-owned namespaces whose exact
upgrade behavior has not been measured on macOS.

| Platform / state                                                                                          | Evidence and key                                                                                                                                                                                                                                                                 | Carries over, or what the person sees                                                                                                                                                                                                                                                                                                                                                                                                                      |
| --------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| macOS notification permission and presentation choices                                                    | Wails `notifications_darwin.go` requires a bundle ID; `notifications_darwin.m` reads `NSBundle.mainBundle.bundleIdentifier` and uses `UNUserNotificationCenter.currentNotificationCenter`. nocx reaches it through `internal/notify/wailsadapter`.                               | **Lost for the new identity:** authorize notifications again. Old delivered/pending entries remain associated with the old app; nocx does not migrate or delete them. OS consequence, not an upgrade experiment here.                                                                                                                                                                                                                                      |
| macOS main-app signing / privacy identity                                                                 | `release.yml` signs the bundle using its `CFBundleIdentifier`; the designated requirement therefore changes.                                                                                                                                                                     | **New identity:** other per-app OS privacy grants may need new consent where applicable. Files & Folders, Full Disk Access and local-network behavior were not measured here; do not promise their carry-over or assert that each will reset.                                                                                                                                                                                                              |
| macOS default webview website data and cache                                                              | Wails `webview_window_darwin.go` creates `WKWebViewConfiguration` without setting `websiteDataStore`. WebKit's default persistent store uses the bundle/process identifier in its website-data and cache directories.                                                            | **Fresh namespace:** web caches, cookies, localStorage and IndexedDB do not follow the old bundle ID. nocx's actual localStorage tenants are a theme bootstrap mirror (`renderers/theme-bootstrap.ts`, reconciled from settings), shell-integration warning silences (`integration/status.ts`, dismissed warnings return), and cell-drift diagnostic state (`scrollback/cell-drift.ts`, flags/samples reset). Exact installed WebKit version not measured. |
| macOS UserDefaults / AppKit preferences                                                                   | Wails reads `standardUserDefaults` for `AppleInterfaceStyle` and `AppleActionOnDoubleClick`. OS per-app defaults use the application domain. No nocx settings writer uses this store.                                                                                            | **New per-app domain:** custom per-app defaults or panel preferences do not follow; global defaults still apply. No claim of a measured loss of product settings.                                                                                                                                                                                                                                                                                          |
| macOS saved application state                                                                             | Wails `application_darwin_delegate.m` supports secure restorable state. macOS saved application state uses the bundle identifier; no nocx frame-autosave name was found.                                                                                                         | **New OS namespace:** old OS restoration state is not migrated. Product window geometry still carries over through `uistate.json`; see the next row.                                                                                                                                                                                                                                                                                                       |
| macOS and Linux window geometry / tabs / layout                                                           | `internal/uistate/uistate.go` names `uistate.json`; the client-host restore path is `main.go` / `windowgeometry.go` (ADR-0098).                                                                                                                                                  | **Carries over:** same document and restore path; not keyed to desktop identity.                                                                                                                                                                                                                                                                                                                                                                           |
| macOS and Linux settings, SSH profiles, vault documents, file secrets, history, workspaces and app caches | `internal/storage/appdir.go` names `nocx`; `paths.go` uses `Library/Application Support/nocx` and `Library/Caches/nocx` on macOS, and XDG config/data/cache directories under `nocx` on Linux. `release`/dev profile selection is unchanged.                                     | **Carries over:** neither ID renames these directories or their documents.                                                                                                                                                                                                                                                                                                                                                                                 |
| macOS keychain and Linux Secret Service secrets / content key                                             | `internal/vault/system/system.go` uses service `nocx` and persisted secret-reference accounts. The coordinator remains separately signed as `com.nocx.server` (`release.yml`, `build/darwin/requirement.txt`). Content-key namespace/salt and machine identity remain unchanged. | **Carries over:** no service/account rename, access-group change or content-key rotation. A bundle-only identity change does not rename the coordinator that accesses secrets.                                                                                                                                                                                                                                                                             |
| macOS and Linux running sessions / coordinator discovery / update journal                                 | `internal/coordinator/server.go` puts runtime discovery under the unchanged data directory. `internal/update/journal.go`'s `bundleID` is device/inode identity, not `CFBundleIdentifier`.                                                                                        | **Carries over:** no desktop-ID-dependent session or journal migration.                                                                                                                                                                                                                                                                                                                                                                                    |
| Linux dock/launcher grouping and per-app compositor rules                                                 | Wayland `app_id` matches the desktop entry basename. GTK application identity also names the GApplication bus registration.                                                                                                                                                      | **New desktop identity:** new `dev.nocx.desktop`; user-pinned old launchers and rules targeting `nocx` or `org.wails.nocx` need re-pinning/reconfiguration if they target the old identity. No core data moves.                                                                                                                                                                                                                                            |
| Linux notifications                                                                                       | Wails `notifications_linux.go` passes `application.Options.Name` (`nocx`) to `org.freedesktop.Notifications.Notify`; no desktop-entry hint is sent. Categories remain `os.UserConfigDir()/nocx/notification-categories.json`; permission methods return true.                    | **Carries over in this code:** app name/category path unchanged; no macOS-style permission prompt introduced. Desktop-specific policies were not measured.                                                                                                                                                                                                                                                                                                 |

On the first launch after updating, the person authorizes notifications again.
The window's page storage starts empty, and a dismissed integration warning
returns. Settings, vault, connections, product window geometry and history
remain. Per-app OS defaults and saved application state use a new namespace,
not a copy of the old one.

No active file/protocol association or login-item setup is configured by nocx.
Conditional entries in the unused plist do not establish an installed handler.
Those are not existing product features whose state this change migrates.

The WebKit mechanism is documented by Apple's
[`websiteDataStore`](https://developer.apple.com/documentation/webkit/wkwebviewconfiguration/websitedatastore)
and [`WKWebsiteDataStore.default()`](<https://developer.apple.com/documentation/webkit/wkwebsitedatastore/default()>).
Upstream WebKit's
[`WebsiteDataStoreCocoa.mm`](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/WebsiteData/Cocoa/WebsiteDataStoreCocoa.mm)
uses `applicationOrProcessIdentifier()` for the default directories. These are
mechanism evidence, not a measurement of every shipped macOS WebKit version.

## What the next person inherits

AD-3 keeps platform identity in the thin desktop host. AD-8 keeps it out of
terminal/session behavior. No transport contract or terminal authority changes.
No accepted ADR, contract or protocol document cites either old application-ID
literal, so no citation is moved by pretending an earlier decision exists.
ADR-0050/0052's separate keychain/coordinator decisions and ADR-0098's window
restore ownership remain valid.

The generator was run with the release identity flags and produced
`CFBundleIdentifier = dev.nocx`. A signed macOS upgrade and its OS consent/state
behavior were not exercised here. The release smoke check is the gate on the
actual assembled app rather than a claim from this generation-only check.
