# ADR-0095 — The launch path raises a window Wails has already built

- **Status:** Accepted
- **Date:** 2026-10-20
- **Decided by:** the worker on the v0.6.1 release blocker, from the failed `build-linux`
  gate (run 39463655845) and from a local reproduction of the crash it reported.
- **Related:** AD-3 (the shell is a thin, swappable host), AD-8 (one owner per behaviour),
  [ADR-0093](0093-the-quit-chord-is-claimed-by-the-window-before-the-webview.md) (the same
  window, asking the same kind of question about who owns a Wails call).

## Context

The v0.6.1 AppImage crashed on every launch on a distribution that is not the one it was
built on, and `build-linux`'s verify step caught it: the `none` mutation died with

```
ASSERTION FAILED: no PTY session opened — tail: cs 0x33 fs 0x0 gs 0x0
```

— a Go register dump, not a failure. All four mutations then failed with `not with the
helper-spawn error`, because the process died before WebKit's own helper processes were ever
spawned. The same verifier against the PUBLISHED v0.6.0 AppImage, in the same container,
passed (`AppImage gate: all checks passed`), so the cause was in `v0.6.0..v0.6.1` and not in
the image.

Reproduced by building the 0.6.1 AppImage the way `release.yml` does and running the binary
under Xvfb:

```
SIGSEGV: segmentation violation
PC=0x7f829514cd04 m=0 sigcode=1 addr=0x18
signal arrived during cgo execution

goroutine 1 [syscall, locked to thread]:
_Cfunc_gtk_application_window_new
  ← application.windowNew           linux_cgo_gtk3.go:1478
  ← (*linuxWebviewWindow).run       webview_window_linux.go:356
  ← (*WebviewWindow).Run            webview_window.go:494
  ← (*WebviewWindow).Show           webview_window.go:514
  ← main.(*wailsShell).Show         main.go:203
  ← main.(*WailsApp).afterLaunch    main.go:779
  ← main.(*WailsApp).ServiceStartup main.go:416
  ← (*App).Run.func1                application.go:677
```

The change that put `afterLaunch` there is `nocx-1ebrs.6`'s fix (commit `437847ac3`), which
moved the window raise out of `main()` — where it was a no-op, because the platform
implementation did not exist yet — and into the startup path, on the correct observation that
the call in `main()` did nothing.

What it did not account for is what `WebviewWindow.Show` does when the platform
implementation is still nil. It is not a show:

```go
func (w *WebviewWindow) Show() Window {
	if globalApplication.impl == nil { return w }
	if w.impl == nil || w.isDestroyed() {
		InvokeSync(w.Run)          // ← this creates the platform window
		return w
	}
	...
}
```

`Run` sets `w.impl` and calls the platform's own `run`, which builds the native window. In
`App.Run`, services start **before** the pending windows are dispatched and **before** the
platform main loop runs (`application.go`: `startup()` — services, then `pendingRun` — then
`a.impl.run()`). So the startup path asks for a GTK window while the `GtkApplication` has not
been registered yet, and GTK3 dereferences NULL: `addr=0x18`, inside `gtk_application_window_new`.

WHY v0.6.0 SURVIVED THE SAME ORDERING. In v0.6.0 nothing asked during `ServiceStartup`; the
window was created by Wails' own pending-window pass. That pass runs `pending.Run()` in a
**goroutine of its own**, so its `InvokeSync` is dispatched through `g_idle_add` rather than
executed inline: the window is built by the main loop, from an idle callback, after
`g_application_run` has registered the application. The defect is therefore not "the window
is created before the main loop" — it is **"the window is created inline, on the main thread,
before the main loop"**, which is what calling `Show` from a service does (`isOnMainThread`
is true there, so `InvokeSync` runs the function directly).

## Decision

**1. The shell raises a window; it never builds one.** `wailsShell.Show` forwards to Wails
only when the window is already on screen — `window.IsVisible()` is the question
`WebviewWindow` answers without creating anything, because it returns false while the
platform implementation is nil. A window that does not exist is not asked for, and Wails
builds it in its own pass a moment later.

**2. The startup path no longer calls `afterLaunch`.** The window is raised and the notice
presented from `ResolveBackend`, the renderer's first call, which cannot arrive until the
webview is loaded and the window is therefore on screen. This is the ordering `nocx-1ebrs.6`
actually needed; the startup call was its unnecessary half.

**3. The rule is stated at the seam, not at the call sites.** There are two callers of the
raise and both would have to remember; `raisableWindow` and the guard in `wailsShell.Show`
make it the shell's own property, and `TestTheShellRaisesOnlyAWindowThePlatformHasBuilt`
drives both halves of it.

## Consequences

- Nothing is lost by not asking early: Wails shows the window itself once built — the GTK3
  window's `run` shows it unless the window is `Hidden`, and the darwin one does so from its
  `WebViewDidFinishNavigation` handler. Both platforms behaved this way in v0.6.0, which is
  what the published AppImage that passed the gate was built from.
- A future second window, or a real "show as soon as it exists" requirement, must reach for
  the same seam rather than calling `WebviewWindow.Show` directly: the call is a
  create-or-show, and its create half is unsafe before the main loop on GTK3.
- The unit test that holds this is cheap because the predicate is a documented Wails
  behaviour, not a guess: no display, no GTK, no cgo run is needed to check it.
