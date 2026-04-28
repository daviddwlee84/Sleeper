# `systemd-inhibit` doesn't silence the GNOME screen lock

**Symptoms** (grep this section): screen still blanks on Ubuntu desktop, screen still locks under sleeper, GNOME screen lock fires, "Screen Lock" 5 minutes, `systemd-inhibit --list` shows the lock but the screen still goes dark, "Blank Screen" still triggers, gnome-shell idle, `org.gnome.ScreenSaver`, `org.freedesktop.ScreenSaver`, lock-enabled screensaver, idle-delay desktop
**First seen**: 2026-04
**Affects**: Ubuntu desktop / Fedora Workstation / any GNOME session; same root cause for KDE Plasma's `org.freedesktop.ScreenSaver` re-implementation
**Status**: documented limitation in v1; deferred GNOME/KDE D-Bus inhibitor tracked as P3 in `TODO.md`

## Symptom

Sleeper is running, holding a `systemd-inhibit` lock. Verification looks good:

```
$ systemd-inhibit --list
WHO     UID USER PID  COMM            WHAT      WHY            MODE
sleeper 1000 you 12345 systemd-inhibit idle:sleep look-busy TUI block
```

Yet five (or whatever the user-configured) minutes later, the screen blanks
and the lock screen comes up. `systemctl suspend` is correctly inhibited; the
laptop *doesn't sleep*. But the **screen does lock**, defeating the whole
"step away from your desk" use case.

## Root cause

logind inhibitors (`systemd-inhibit`) and GNOME's screen lock are **two
separate idle-detection systems** that don't talk to each other:

| Layer | API | What it controls |
|-------|-----|------------------|
| logind | `systemd-inhibit --what=idle:sleep` | system suspend, lid handler, idle-triggered sleep |
| gnome-shell | `org.gnome.ScreenSaver` (D-Bus) | screen blank, lock screen, *Privacy → Screen Lock* delay |

`--what=idle` blocks logind from *reporting* the system as idle, but
gnome-shell runs its own input-idle timer entirely in user space. It doesn't
consult logind, doesn't honour the systemd inhibitor, and locks the screen
based on the gsetting `org.gnome.desktop.session/idle-delay`.

KDE Plasma re-implements `org.freedesktop.ScreenSaver` with the same
isolation. X11 sessions add a third layer (DPMS / X screensaver), which
*sometimes* respects xdg-screensaver but inconsistently.

## Workaround

User-side, no code change. Disable the GNOME screen lock for the period
sleeper is running:

```bash
gsettings set org.gnome.desktop.screensaver lock-enabled false
gsettings set org.gnome.desktop.session idle-delay 0
```

Reverse afterwards:

```bash
gsettings reset org.gnome.desktop.screensaver lock-enabled
gsettings reset org.gnome.desktop.session idle-delay
```

Or via *Settings → Privacy → Screen Lock → Blank Screen → Never*.

## Why we don't fix this in v1

A real fix means calling `org.gnome.SessionManager.Inhibit` (or
`org.freedesktop.ScreenSaver.Inhibit` for the cross-DE portal) over D-Bus.
That requires either:

- a Go D-Bus client dep (`github.com/godbus/dbus`), inflating the dependency
  graph for a single-DE feature, or
- shelling out to `gnome-session-inhibit` per DE, which only ships with GNOME
  and has no KDE/XFCE/Cinnamon equivalent.

For a "look busy" toy this isn't worth it. The `gsettings` workaround is
two commands and reversible. Promote when a real Ubuntu-desktop user files
a bug.

## Prevention

- Don't claim "keeps your screen awake on Linux" without the GNOME caveat.
  README documents both the systemd-inhibit feature and this gap so users
  aren't surprised.
- TTY / SSH / headless users are unaffected — there's no gnome-shell to fight.

## Related

- Sibling pitfalls: `pitfalls/caffeinate-u-flag-silent-exit.md` (the macOS
  side of the same "supervised child silently exits" failure mode).
- Source: `internal/caffeinate/caffeinate_linux.go::Start`; design notes in
  `.claude/plans/linux-feature-os-ubuntu-enumerated-pearl.md`.
- Upstream: GNOME bugzilla has long discussions about logind ↔ gnome-shell
  inhibitor unification; nothing has shipped.
