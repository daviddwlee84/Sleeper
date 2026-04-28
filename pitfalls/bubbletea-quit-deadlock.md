# SIGTERM to a Bubble Tea program leaks the child process — `prog.Quit()` is non-blocking

**Symptoms** (grep this section): caffeinate leaks, child process orphaned, sleeper hangs on SIGTERM, `prog.Quit() doesn't return`, Bubble Tea renderer wedged, `pgrep caffeinate` still shows pid after parent exits, signal handler returns immediately, defer didn't fire
**First seen**: 2026-04
**Affects**: any Bubble Tea v1 program that spawns a child process and relies on `defer` to clean it up
**Status**: fixed via 1.5 s hard-fallback deadline in the signal goroutine

## Symptom

Test harness (Python via PTY):

```python
proc = subprocess.Popen(["sleeper", "--project", "."], ...)
time.sleep(0.5)
os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
proc.wait(timeout=2)  # ← TimeoutExpired
```

After the test bails out:

```
$ pgrep caffeinate
51104              # ← still alive, hours later
```

sleeper itself is also still alive — a signal handler caught the
SIGTERM, called `prog.Quit()`, but the program never returned and the
deferred `caffeinate.Stop()` never ran.

## Root cause

`bubbletea.Program.Quit()` sends a quit message to the dispatcher and
returns immediately:

```go
// (paraphrased from upstream)
func (p *Program) Quit() {
    p.Send(QuitMsg{})
}
```

If the dispatcher's render loop is wedged (busy applying a huge frame,
blocked on a channel, GC-pause-storming), the `QuitMsg` sits in the
queue. `prog.Run()` doesn't return. `defer cmd.Process.Kill()` in main
doesn't run. The child process — caffeinate, with `Setpgid: true`, in
its own process group, immune to our signal — stays alive.

This is also the *correct* behaviour: `Quit` is "request a graceful
shutdown," not "tear down right now." There's no public guarantee about
how long it takes.

## Workaround

Hard-fallback deadline in the signal goroutine. After requesting
graceful quit, give the runtime a fixed budget; if it doesn't return,
clean up children manually and `os.Exit`:

```go
done := make(chan struct{})
go func() {
    ch := make(chan os.Signal, 1)
    signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
    select {
    case <-ch:
        prog.Quit()
        select {
        case <-done:
            // Run() returned cleanly; main's defers will run.
        case <-time.After(1500 * time.Millisecond):
            // Bubble Tea is wedged. Kill children ourselves.
            if caf != nil { _ = caf.Stop() }
            _ = prog.ReleaseTerminal()
            os.Exit(130)
        }
    case <-done:
    }
}()
_, runErr := prog.Run()
close(done)
return runErr
```

Notes:
- `done` is closed AFTER `prog.Run()` returns, so the signal goroutine
  sees both paths.
- `prog.ReleaseTerminal()` restores cooked mode + main screen so the
  user's shell isn't left garbled.
- `os.Exit(130)` is the conventional "killed by SIGINT" exit code; works
  for SIGTERM too in practice.

## Prevention

- **Whenever you spawn a child whose lifetime is tied to a Bubble Tea
  program, also install a deadlined signal handler** like the one
  above. The pattern is small enough to copy-paste.
- For child processes specifically: spawn with `Setpgid: true` and use
  `syscall.Kill(-pgid, SIGTERM)` for shutdown — it kills the whole
  process group cleanly. Plain `cmd.Process.Kill()` only signals the
  immediate child and leaves grandchildren orphaned.
- Add an integration test that SIGTERMs the binary and asserts
  `pgrep <child-name>` is empty within N seconds. Without the test,
  this regresses the moment someone "tidies up" the signal handler.

## Related

- Sibling pitfalls: `pitfalls/caffeinate-u-flag-silent-exit.md` (the
  *other* reason caffeinate "disappears").
- Source: `cmd/sleeper/main.go` signal goroutine; commit `696ef00`.
- Upstream: `github.com/charmbracelet/bubbletea` `program.go::Quit`.
