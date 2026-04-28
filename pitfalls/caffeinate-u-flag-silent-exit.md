# `caffeinate` child silently exits after 5 seconds — the `-u` flag

**Symptoms** (grep this section): caffeinate dies, screen still locks, `pgrep caffeinate` empty after a few seconds, sleeper still running, `caffeinate -dimsu`, `-u`, "user activity assertion", display sleeps despite caffeinate being launched
**First seen**: 2026-04
**Affects**: macOS `caffeinate(8)` (all versions); any wrapper that spawns it without `-t`
**Status**: fixed by removing `-u`

## Symptom

A long-running supervisor spawns `caffeinate -dimsu` as a child to keep
the screen awake, expects it to live for the whole session. Reality:

```
$ caffeinate -dimsu &
[1] 12345
$ sleep 6 && pgrep caffeinate
$ # ← empty; child is gone
```

In sleeper, the screen would still lock after the screensaver timeout
even though the process believed it had pinned `caffeinate` for the
whole session.

## Root cause

`man 8 caffeinate`:

> `-u` Declare that the user is active. If the display is off, this
> option turns the display on and prevents it from going into idle sleep
> until a timeout expires. **If no timeout is specified with `-t`
> option, a default of 5 seconds is used.**

The `-d`, `-i`, `-m`, `-s` flags are *assertion-based* and live as long
as the process. `-u` is *event-based* — it issues a single user-activity
notification, then exits when the timeout (default 5 s) fires. Mixing
`-u` with the assertion flags doesn't extend `-u`'s lifetime; the whole
process exits when `-u`'s timer is up.

## Workaround

Drop `-u`:

```diff
- cmd := exec.Command("caffeinate", "-dimsu")
+ cmd := exec.Command("caffeinate", "-dims")
```

`-dims` keeps display, idle, disk, and system-on-AC awake for as long as
the process is running. If you genuinely want a one-shot user-activity
ping, use `-u -t 0` and a separate long-running `caffeinate -dims`.

Verification:

```sh
caffeinate -dims & PID=$!
sleep 6
kill -0 $PID && echo "still alive ✓" || echo "dead ✗"
kill -TERM $PID
```

## Prevention

- When wrapping `caffeinate`, use only the assertion flags (`-d`/`-i`/`-m`/`-s`)
  unless you specifically need the user-activity event.
- Add a smoke test that asserts the supervised child is alive after
  `>5s`; this would have caught the bug at CI time.

## Related

- Sibling pitfalls: `pitfalls/bubbletea-quit-deadlock.md` (cleanup
  guarantees for the same `caffeinate` child on the way out).
- Source: `internal/caffeinate/caffeinate.go::Start`; commit `696ef00`.
- Apple manpage: `man 8 caffeinate`.
