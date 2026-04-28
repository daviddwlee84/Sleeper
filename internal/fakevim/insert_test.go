package fakevim

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestDetectIndent(t *testing.T) {
	lines := []string{
		"package main",
		"",
		"func foo() {",
		"    if true {",
		"        x := 1",
		"",
		"        y := 2",
		"    }",
		"}",
	}
	cases := []struct {
		row  int
		want string
	}{
		{0, ""},
		{2, ""},
		{3, "    "},
		{4, "        "},
		{5, "        "}, // blank — looks back to row 4
		{6, "        "},
		{7, "    "},
		{8, ""},
	}
	for _, tc := range cases {
		got := detectIndent(lines, tc.row)
		if got != tc.want {
			t.Errorf("detectIndent(row=%d) = %q; want %q", tc.row, got, tc.want)
		}
	}
}

func TestIsCleanEnd(t *testing.T) {
	clean := []string{"", "   ", "}", ");", "x;", "y,", "  }", "func() {}"}
	dirty := []string{"x := 1", "if foo {", "// comment", "package main"}
	for _, s := range clean {
		if !isCleanEnd(s) {
			t.Errorf("isCleanEnd(%q) = false; want true", s)
		}
	}
	for _, s := range dirty {
		if isCleanEnd(s) {
			t.Errorf("isCleanEnd(%q) = true; want false", s)
		}
	}
}

func TestPickWordBoundary_NeverInsideToken(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	line := "    foo := bar(quux)"
	// repeat picks; every result must be either 0, EOL, or right after whitespace
	for i := 0; i < 200; i++ {
		c := pickWordBoundary(line, rng)
		if c == 0 || c == len(line) {
			continue
		}
		// must be at the start of a word: previous char is whitespace
		prev := line[c-1]
		if prev != ' ' && prev != '\t' {
			t.Errorf("pickWordBoundary returned mid-token col=%d (prev=%q) for %q", c, string(prev), line)
		}
	}
}

func TestPickSafeRow_PrefersCleanLines(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	lines := []string{
		"package main",      // 0  dirty (ends 'n')
		"",                  // 1  CLEAN (blank)
		"func foo() {",      // 2  dirty (ends '{')
		"    bar()",         // 3  CLEAN (ends ')')
		"    return nil",    // 4  dirty (ends 'l')
		"}",                 // 5  CLEAN (ends '}')
		"",                  // 6  CLEAN (blank)
		"func baz(x int) {", // 7  dirty (ends '{')
	}
	cleanRows := map[int]bool{1: true, 3: true, 5: true, 6: true}
	hits := 0
	for i := 0; i < 200; i++ {
		got := pickSafeRow(lines, 4, rng)
		if cleanRows[got] {
			hits++
		}
	}
	// pickSafeRow MUST always pick a clean row when at least one exists in window
	if hits != 200 {
		t.Errorf("expected 200/200 clean-row picks (window has %d clean rows); got %d", len(cleanRows), hits)
	}
}

func TestInsertBank_AllSingleLine(t *testing.T) {
	for lang, bank := range insertBank {
		for _, p := range bank {
			if strings.ContainsRune(p, '\n') {
				t.Errorf("insertBank[%s] contains multi-line phrase: %q", lang, p)
			}
		}
	}
}
