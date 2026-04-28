# Process eats GBs/sec, system swaps, macOS reboots — `os.ReadFile("/dev/urandom")`

**Symptoms** (grep this section): RAM 飆高, system unresponsive, macOS auto reboot, `heap=449MB sys=1194MB ... heap=11741MB` in debug log over 17s, single-goroutine allocation runaway, `/dev/urandom`, `os.ReadFile`, `io.ReadAll`, `io.Copy`, character device, no EOF
**First seen**: 2026-04
**Affects**: any Go code that calls `os.ReadFile`, `io.ReadAll`, or `io.Copy` on a character device or pipe with no natural EOF; macOS, Linux
**Status**: fixed in scanner; pattern documented here as a class of bug

## Symptom

`sleeper --project . --debug-log /tmp/sleeper.log` (no `--seed`) hung with
no TUI ever drawing. Activity Monitor showed `sleeper` at multiple GB
RES, system swap climbing through 6 GB, eventually macOS rebooted on its
own.

`/tmp/sleeper.log` (verbatim):

```
2026/04/28 15:38:56.263800 debug-log started; pid=58963
2026/04/28 15:38:57.269645 heap=449MB sys=1194MB stacks=608KB goroutines=2 numGC=33
2026/04/28 15:38:58.272891 heap=876MB sys=2776MB stacks=608KB goroutines=2 numGC=37
2026/04/28 15:39:00.279479 heap=1710MB sys=4490MB stacks=608KB goroutines=2 numGC=43
2026/04/28 15:39:17.174456 heap=11741MB sys=17039MB stacks=608KB goroutines=2 numGC=60
```

Key tells:
- **goroutines stays at 2**: only main + the debug-log goroutine. Bubble
  Tea hasn't started yet, so the explosion is in code that runs
  *synchronously before* `prog.Run()`.
- **heap grows ~1 GB/sec** while `numGC` keeps incrementing — GC is
  running normally but new allocations are coming faster than reclaim.
- A single goroutine cannot drive that much allocation unless one call
  is itself unbounded.

Reproduction:

1. `sleeper --project anything` without `--seed`
2. Within 1–2 seconds, watch RSS in `top` or Activity Monitor

## Root cause

`internal/scanner/scanner.go::randomSeed()` had:

```go
func randomSeed() uint64 {
    var b [8]byte
    _, _ = os.ReadFile("/dev/urandom") // best effort, unused
    for i := range b {
        b[i] = byte(os.Getpid() >> uint(i))
    }
    return uint64(b[0]) | ...
}
```

`os.ReadFile` is implemented as: stat the file → allocate a buffer of
that size → `io.ReadAll` if stat is unavailable → keep growing the
buffer in `2N` doublings until EOF.

`/dev/urandom` is a **character device**: stat reports size 0, fallback
path is `io.ReadAll`, which **never sees EOF** — `/dev/urandom` is an
infinite stream of bytes by design. The buffer doubles past 1 GB → 2 GB
→ 4 GB → 8 GB → ..., GC reclaims the previous size as the next one
allocates, but the working set explodes either way. macOS swap fills
within ~15 s, the watchdog gives up, system reboots.

The `_, _ = ` discard hid the bug from `go vet`, and the comment
"best effort, unused" was about the *return value* — the I/O itself
still happens.

## Workaround

```go
import (
    cryptorand "crypto/rand"
    "encoding/binary"
)

func randomSeed() uint64 {
    var b [8]byte
    if _, err := cryptorand.Read(b[:]); err != nil {
        return uint64(os.Getpid())*0x9e3779b97f4a7c15 ^ uint64(os.Getppid())
    }
    return binary.LittleEndian.Uint64(b[:])
}
```

`crypto/rand.Read` reads exactly the requested length into the buffer
and returns. No infinite-stream trap.

## Prevention

- **Hard rule**: never use `os.ReadFile`, `io.ReadAll`, or `io.Copy(buf, src)`
  with `src` that isn't a regular bounded file. Character devices
  (`/dev/urandom`, `/dev/zero`, `/dev/null` on read-side weirdness),
  pipes, sockets, growing log files all qualify. Use the typed APIs:
  `crypto/rand.Read(buf)`, `io.ReadFull(src, buf)`, or a `bufio.Reader`
  with explicit byte limits.
- **Process safety net**: `runtime/debug.SetMemoryLimit(N)` in `main()`
  caps the Go heap so even if a future bug ALLOCates without bound, the
  process OOMs cleanly long before swap fills. sleeper's `main()` sets
  256 MB by default; configurable via `--mem-limit`.
- **Debug-log scaffolding**: `--debug-log <path>` writes per-second
  memstats. The `goroutines=2` line above is what made this diagnosable
  without re-crashing the machine. Keep that flag.

## Related

- TODO entry: none — fix is shipped.
- Sibling pitfalls: `pitfalls/bubbletea-quit-deadlock.md` (the safety-net
  fallback path that caps damage from any future runaway).
- Source: `internal/scanner/scanner.go` `randomSeed`; commit `64e82d4`.
