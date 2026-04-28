package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
}

func parseFlags() opts {
	cwd, _ := os.Getwd()
	o := opts{}
	flag.StringVar(&o.project, "project", cwd, "Path to the real project to fake-edit")
	flag.StringVar(&o.scene, "scene", "vim+shell", "Initial scene: vim|shell|vim+shell")
	flag.DurationVar(&o.tickDur, "tick", 150*time.Millisecond, "Base animation tick")
	flag.BoolVar(&o.noCaffeinate, "no-caffeinate", false, "Do not start caffeinate (debug)")
	flag.BoolVar(&o.noTUI, "no-tui", false, "Skip TUI; just hold caffeinate until SIGINT (debug)")
	flag.StringVar(&o.editor, "editor", os.Getenv("EDITOR"), "Editor for handover (default $EDITOR)")
	flag.Int64Var(&o.seed, "seed", 0, "Deterministic RNG seed (0 = random)")
	flag.StringVar(&o.aiStyle, "ai-style", "mixed", "AI conversation style: debug|feature|refactor|mixed")
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

func run() (err error) {
	o := parseFlags()

	scn, err := scanner.New(o.project, uint64(o.seed))
	if err != nil {
		return fmt.Errorf("scan project: %w", err)
	}

	var caf *caffeinate.Manager
	if !o.noCaffeinate {
		c, cerr := caffeinate.Start()
		if cerr != nil {
			return fmt.Errorf("caffeinate: %w", cerr)
		}
		caf = c
		fmt.Fprintf(os.Stderr, "[sleeper] caffeinate pid=%d holding the screen awake\n", caf.PID())
	}
	defer func() {
		if caf != nil {
			_ = caf.Stop()
		}
	}()

	if o.noTUI {
		fmt.Fprintln(os.Stderr, "[sleeper] --no-tui mode; press Ctrl+C to exit")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		<-ch
		return nil
	}

	root, err := scene.New(scn, scene.Options{
		Editor:        o.editor,
		InitialLayout: parseLayout(o.scene),
		AIStyle:       o.aiStyle,
	}, uint64(o.seed))
	if err != nil {
		return fmt.Errorf("scene: %w", err)
	}

	prog := tea.NewProgram(root, tea.WithAltScreen())

	// Signal handler: forward SIGTERM/SIGHUP into a graceful Quit so the defer
	// cleanup of caffeinate still runs. SIGINT is already handled by Bubble Tea.
	done := make(chan struct{})
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGHUP)
		select {
		case <-ch:
			prog.Quit()
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
