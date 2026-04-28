package fakevim

import "testing"

func TestExpandTabs(t *testing.T) {
	cases := []struct {
		in, want string
		size     int
	}{
		{"", "", 4},
		{"abc", "abc", 4},
		{"\t", "    ", 4},
		{"\tx", "    x", 4},
		// tab-stop alignment: 'a' at col 0, then \t advances to col 4
		{"a\tb", "a   b", 4},
		// 'ab' -> col 2, tab fills 2 spaces to col 4
		{"ab\tc", "ab  c", 4},
		// 'abcd' -> col 4, tab fills full 4 spaces to col 8
		{"abcd\tx", "abcd    x", 4},
		// multiple tabs
		{"\t\t", "        ", 4},
		// non-zero tabSize fallback
		{"\t", "        ", 8},
		// tabSize <= 0 falls back to 4
		{"\t", "    ", 0},
	}
	for _, tc := range cases {
		got := expandTabs(tc.in, tc.size)
		if got != tc.want {
			t.Errorf("expandTabs(%q, %d) = %q; want %q", tc.in, tc.size, got, tc.want)
		}
	}
}

func TestExpandTabs_NoTab_ReturnsSame(t *testing.T) {
	in := "package main\nimport \"fmt\"\n"
	if expandTabs(in, 4) != in {
		t.Errorf("expandTabs should be a no-op on tab-free input")
	}
}
