# Linux desktop identity

The installed identity is `dev.nocx`. A build without `-tags release` uses
`dev.nocx-dev`. The display name and executable remain `nocx`.

Two public setters are required. They do not answer the same question:

- Wails `Linux.ProgramName` calls `g_set_prgname`. GTK3's Wayland backend
  uses that value for `xdg_toplevel.set_app_id`; it also affects X11 grouping.
- The first startup service uses `g_application_set_application_id` on the
  default GtkApplication. Wails beta.9 constructs it before starting services,
  then registers it in `linuxApp.run`, after services return. The adapter
  refuses a missing or registered application and reads the ID back.

Wails beta.9 has no application-ID option. No dependency patch is needed.
GLib documents the public setter's pre-registration constraint:
<https://docs.gtk.org/gio/method.Application.set_application_id.html>.
GTK3's program-name mapping is in
[`gdk_wayland_window_create_xdg_toplevel`](https://github.com/GNOME/gtk/blob/3.24.52/gdk/wayland/gdkwindow-wayland.c).

## Checks

```sh
go test ./internal/desktopidentity
go test -tags release ./internal/desktopidentity
go test -tags gtk3 -run 'TestLinuxDesktopIdentityWiring|TestAppImageDesktopIdentity' .
sh scripts/install/test-install.sh
```

The GLib checks run in child processes because default-application ownership is
process-global. They cover a missing application, the setter before registration,
ID retention through registration, and refusal after registration.
The installer check verifies `dev.nocx.desktop` and `StartupWMClass=dev.nocx`,
including rollback on replacement failures. No migration of older launchers is
performed.

## Runtime measurement (2026-10-24)

The actual nocx GTK3 executable was launched against native headless Weston 16
with `GDK_BACKEND=wayland` and `WAYLAND_DEBUG=1`, using disposable home directories.
`testdata/observe-application.c` was loaded as a temporary `LD_PRELOAD` observer.
It reads the GApplication ID before `g_application_run` and from its `startup`
signal after registration. It does not change identity. Compile it with:

```sh
cc -shared -fPIC $(pkg-config --cflags --libs gio-2.0) \
  internal/desktopidentity/testdata/observe-application.c -ldl -o observer.so
```

For an existing Wayland compositor, build the release shell, then launch it
with both diagnostics enabled (and an isolated home):

```sh
go build -tags 'gtk3 release' -o build/bin/nocx .
probe_home="$(mktemp -d)"
HOME="$probe_home" GDK_BACKEND=wayland WAYLAND_DEBUG=1 \
  LD_PRELOAD="$PWD/observer.so" build/bin/nocx
```

Read `IDENTITY before-run registered=0`, `IDENTITY registered=1`, and the
`xdg_toplevel.set_app_id` request. A window that starts is not enough.

| Build / mutation                                    | GApplication ID before / after registration | Program name   | Wayland `set_app_id` |
| --------------------------------------------------- | ------------------------------------------- | -------------- | -------------------- |
| Control: original program name, no identity service | `org.wails.nocx` / same                     | `nocx`         | `nocx`               |
| GLib service only; original program name            | `dev.nocx` / same                           | `nocx`         | `nocx`               |
| Program name only; no identity service              | `org.wails.nocx` / same                     | `dev.nocx`     | `dev.nocx`           |
| Release: both setters                               | `dev.nocx` / same                           | `dev.nocx`     | `dev.nocx`           |
| Development: both setters                           | `dev.nocx-dev` / same                       | `dev.nocx-dev` | `dev.nocx-dev`       |

All five configurations stayed running, registered their GtkApplication, and sent
Wayland surface commits. The first trial disproved the assumption that changing
the GtkApplication ID alone changes the Wayland identity. The control also proves
that the hardcoded Wails GtkApplication ID was not itself the Wayland ID.
Wails uses `G_APPLICATION_NON_UNIQUE`, so a unique D-Bus service name is not an
identity check here.

Limits: the measurement used a locally built GTK3 executable, not an installed
AppImage. The window opened, but its adjacent coordinator was absent; terminal
functionality was not measured. GTK4 was not available for runtime measurement.
Installed-launcher naming was checked by the installer fixture, not by desktop
shell grouping. The release smoke gate still owns the packaged-product check.

The isolated runtime homes also confirm that WebKitGTK's default website data
and cache directories follow the program name: `.local/share/nocx` and
`.cache/nocx` in the control and GLib-only runs become
`.local/share/dev.nocx` and `.cache/dev.nocx` for the release, and
`dev.nocx-dev` for development. These contain WebKit storage, media keys,
`WebKitCache`, and `CacheStorage`. They are not nocx's core profile selector,
which remains `nocx` / `nocx-dev`. No website-data migration is performed.
