// memprobe drives the scene model with synthesized ticks at a realistic
// terminal size and prints RSS / heap stats over time.
//
// Two modes:
//   - default: pump ticks directly into Update (fast, no real Cmd execution)
//   - exec: also evaluate every returned Cmd (closer to real Bubble Tea behavior)
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sleeper/internal/scanner"
	"github.com/daviddwlee84/sleeper/internal/scene"
)

func main() {
	root := flag.String("project", ".", "project to scan")
	dur := flag.Duration("dur", 20*time.Second, "how long to drive the model")
	exec := flag.Bool("exec", false, "execute returned cmds in goroutines (drains them too)")
	w := flag.Int("w", 200, "terminal width")
	h := flag.Int("h", 60, "terminal height")
	flag.Parse()

	scn, err := scanner.New(*root, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		os.Exit(1)
	}
	mdl, err := scene.New(scn, scene.Options{
		Editor:        "vi",
		InitialLayout: scene.LayoutVimAI,
		AIStyle:       "mixed",
	}, 42)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scene: %v\n", err)
		os.Exit(1)
	}
	mm, _ := mdl.Update(tea.WindowSizeMsg{Width: *w, Height: *h})
	model := mm.(scene.Model)

	start := time.Now()
	report := func(tag string) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		fmt.Printf("[%5.1fs] %s heap=%dMB sys=%dMB stacks=%dKB goroutines=%d numGC=%d\n",
			time.Since(start).Seconds(), tag,
			ms.HeapAlloc/1024/1024, ms.Sys/1024/1024,
			ms.StackInuse/1024, runtime.NumGoroutine(), ms.NumGC)
	}
	report("start")

	// queue of pending msgs to feed back into Update.
	msgQ := make(chan tea.Msg, 1024)

	// seed: each pane's Init returns a tick. Run them so a TickMsg appears.
	enqueueCmd(msgQ, model.Init(), *exec)

	deadline := time.Now().Add(*dur)
	step := 0
	for time.Now().Before(deadline) {
		select {
		case msg := <-msgQ:
			mm, cmd := model.Update(msg)
			model = mm.(scene.Model)
			enqueueCmd(msgQ, cmd, *exec)
			_ = model.View()
			step++
		default:
			// no msg yet — let timers fire
			time.Sleep(2 * time.Millisecond)
		}
		if step%500 == 0 && step > 0 {
			report(fmt.Sprintf("step=%d qlen=%d", step, len(msgQ)))
		}
	}
	report("end")
	fmt.Printf("total steps: %d (qlen=%d)\n", step, len(msgQ))
}

// enqueueCmd flattens a Cmd, spawning a goroutine for each leaf so that
// timer-based commands actually fire. Resulting msgs go to msgQ.
func enqueueCmd(q chan tea.Msg, c tea.Cmd, exec bool) {
	if c == nil {
		return
	}
	if !exec {
		return
	}
	go func() {
		msg := c()
		if msg == nil {
			return
		}
		// tea.Batch hides batched cmds inside a private msg type; we can't
		// easily unwrap. Approximation: push the msg directly. This means
		// batched commands are coalesced into one; not perfect but good enough.
		q <- msg
	}()
}
