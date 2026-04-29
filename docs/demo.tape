# sleeper demo recording — driven by VHS (https://github.com/charmbracelet/vhs)
#
# Render with:
#   vhs docs/demo.tape
#
# Produces docs/demo.gif (embedded in README) and docs/demo.webm
# (higher-quality fallback for external links). See docs/demo-recording.md
# for the full how-to and the reasoning behind these settings.

Output docs/demo.gif
Output docs/demo.webm

Set Shell        "zsh"
Set FontSize     14
Set Width        1100
Set Height       700
Set Theme        "Catppuccin Mocha"
Set TypingSpeed  60ms
Set PlaybackSpeed 1.0

# Build the binary off-camera so we don't depend on whatever PATH the
# VHS-spawned shell happens to inherit. /tmp/sleeper is throwaway.
Hide
Type "cd /Volumes/Data/Program/tries/2026-04-28-sleep-helper && go build -o /tmp/sleeper ./cmd/sleeper"
Enter
Sleep 5s
Type "clear"
Enter
Show

# --seed 42        deterministic layout/file pick so re-recordings match
# --tick 180ms     a hair slower than default (150ms); easier on the eye
# --no-caffeinate  skip caffeinate inside VHS (no real screen to keep awake)
Type "/tmp/sleeper --project . --seed 42 --tick 180ms --no-caffeinate"
Enter

# Default scene: vim+shell. Let it breathe so viewers see syntax highlighting,
# fake cursor jitter, and a real `git status` cycle through.
Sleep 7s

# Tab → vim+ai. Show the chat-bubble pane.
Tab
Sleep 6s

# Tab → vim only. Full-screen the fake editor so the syntax-highlighted
# code is unambiguous.
Tab
Sleep 6s

# Tab → shell only. Show the safe-cmd allowlist output on its own.
Tab
Sleep 5s

# Esc Esc → panic key. Switches back to a plain `git status`-looking
# scene and quietly quits. Also our cleanest end-of-recording.
Escape
Escape
Sleep 1500ms
