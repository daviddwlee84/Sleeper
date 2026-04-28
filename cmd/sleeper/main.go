package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/daviddwlee84/sleeper/internal/caffeinate"
	"github.com/daviddwlee84/sleeper/internal/scanner"
	"github.com/daviddwlee84/sleeper/internal/scene"
)

type opts struct {
	project      string
	scene        string
	tickDur      time.Duration
	noCaffeinate bool
	noTUI        bool
	editor       string
	seed         int64
	aiStyle      string
	memLimitMB   int
	debugLog     string
}

func parseFlags() opts {
	cwd, _ := os.Getwd()
	o := opts{}
	flag.StringVar(&o.project, "project", cwd, "Path to the real project to fake-edit")
	flag.StringVar(&o.scene, "scene", "vim+shell", "Initial scene: vim|shell|vim+shell|vim+ai")
	flag.DurationVar(&o.tickDur, "tick", 150*time.Millisecond, "Base animation tick")
	flag.BoolVar(&o.noCaffeinate, "no-caffeinate", false, "Do not start caffeinate (debug)")
	flag.BoolVar(&o.noTUI, "no-tui", false, "Skip TUI; just hold caffeinate until SIGINT (debug)")
	flag.StringVar(&o.editor, "editor", os.Getenv("EDITOR"), "Editor for handover (default $EDITOR)")
	flag.Int64Var(&o.seed, "seed", 0, "Deterministic RNG seed (0 = random)")
	flag.StringVar(&o.aiStyle, "ai-style", "mixed", "AI conversation style: debug|feature|refactor|mixed")
	flag.IntVar(&o.memLimitMB, "mem-limit", 256, "Hard process memory cap (MB) — safety net against runaway allocations")
	flag.StringVar(&o.debugLog, "debug-log", "", "If set, write per-second memstats to this file")
	flag.Parse()
	return o
}

func parseLayout(s string) scene.Layout {
	switch s {
	case "vim":
		return scene.LayoutVimOnly
	case "shell":
		return scene.LayoutShellOnly
	case "vim+shell":
		return scene.LayoutVimShell
	case "vim+ai":
		return scene.LayoutVimAI
	default:
		return scene.LayoutVimShell
	}
}

// installMemorySafetyNet caps the Go heap so a runaway allocation cannot take
// the whole machine down. The Go runtime will trigger more aggressive GC as we
// approach the limit, then OOM the process (cleanly, with traceback) if we
// blow through it. This is the difference between "sleeper crashes" and
// "macOS reboots."
func installMemorySafetyNet(mb int) {
	if mb <= 0 {
		return
	}
	debug.SetMemoryLimit(int64(mb) * 1024 * 1024)
	// Make GC more aggressive by default so we don't sit close to the limit.
	debug.SetGCPercent(50)
}

// startMemLogger writes runtime memstats to a file every second. Useful for
// hunting "RAM spike on launch" without an interactive debugger.
func startMemLogger(path string, stop <-chan struct{}) error {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	logger := log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	logger.Printf("debug-log started; pid=%d", os.Getpid())
	go func() {
		defer f.Close()
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				logger.Printf("debug-log stopped")
				return
			case <-t.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				logger.Printf("heap=%dMB sys=%dMB stacks=%dKB goroutines=%d numGC=%d",
					ms.HeapAlloc/1024/1024, ms.Sys/1024/1024,
					ms.StackInuse/1024, runtime.NumGoroutine(), ms.NumGC)
			}
		}
	}()
	return nil
}

func run() (err error) {
	o := parseFlags()

	installMemorySafetyNet(o.memLimitMB)

	debugStop := make(chan struct{})
	if err := startMemLogger(o.debugLog, debugStop); err != nil {
		return fmt.Errorf("debug log: %w", err)
	}
	defer close(debugStop)

	scn, err := scanner.New(o.project, uint64(o.seed))
	if err != nil {
		return fmt.Errorf("scan project: %w", err)
	}

	var caf *caffeinate.Manager
	if !o.noCaffeinate {
		c, cerr := caffeinate.Start()
		switch {
		case cerr == nil:
			caf = c
		case errors.Is(cerr, caffeinate.ErrUnsupported):
			fmt.Fprintf(os.Stderr, "[sleeper] %v; running as animated CLI only\n", cerr)
		default:
			return fmt.Errorf("caffeinate: %w", cerr)
		}
	}
	defer func() {
		if caf != nil {
			_ = caf.Stop()
		}
	}()

	if o.noTUI {
		if caf != nil {
			fmt.Fprintf(os.Stderr, "[sleeper] caffeinate pid=%d holding the screen awake\n", caf.PID())
		}
		fmt.Fprintln(os.Stderr, "[sleeper] --no-tui mode; press Ctrl+C to exit")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		<-ch
		return nil
	}

	// Pin the lipgloss color profile to ANSI256 *before* constructing any
	// styles. Without this, lipgloss auto-detects the terminal's profile
	// (often TrueColor under iTerm2) and re-emits styles in 24-bit form.
	// Combined with chroma's terminal256 output, that means every line is
	// double-color-coded — bigger strings, more termenv work per render,
	// and on some terminals: catastrophic memory pressure.
	lipgloss.SetColorProfile(termenv.ANSI256)

	root, err := scene.New(scn, scene.Options{
		Editor:        o.editor,
		InitialLayout: parseLayout(o.scene),
		AIStyle:       o.aiStyle,
	}, uint64(o.seed))
	if err != nil {
		return fmt.Errorf("scene: %w", err)
	}

	prog := tea.NewProgram(root, tea.WithAltScreen())

	// Signal handler: forward SIGTERM/SIGHUP into a graceful Quit so the
	// defer cleanup of caffeinate runs. SIGINT is also handled by Bubble Tea
	// but we hook it too so we can enforce a hard deadline.
	//
	// Hard fallback: if Bubble Tea doesn't yield within 1.5s (renderer
	// wedged, dispatcher blocked), we kill caffeinate ourselves and exit.
	// Without this, a stuck TUI leaves caffeinate running and the screen
	// awake forever.
	done := make(chan struct{})
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		select {
		case <-ch:
			prog.Quit()
			select {
			case <-done:
				// Run() returned cleanly, defers will fire.
			case <-time.After(1500 * time.Millisecond):
				// Bubble Tea is stuck. Force-cleanup and exit.
				if caf != nil {
					_ = caf.Stop()
				}
				_ = prog.ReleaseTerminal()
				fmt.Fprintln(os.Stderr, "[sleeper] forced exit after stuck TUI")
				os.Exit(130)
			}
		case <-done:
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			prog.ReleaseTerminal() //nolint:errcheck
			panic(r)
		}
	}()

	_, runErr := prog.Run()
	close(done)
	return runErr
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sleeper: %v\n", err)
		os.Exit(1)
	}
}
