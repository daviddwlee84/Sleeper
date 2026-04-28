package fakevim

import (
	"bytes"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	style       = styles.Get("monokai")
	formatter   = formatters.Get("terminal256")
	lexerCache  = sync.Map{} // map[string]chroma.Lexer
	bufPool     = sync.Pool{New: func() any { return &bytes.Buffer{} }}
	chromaReady bool
)

func init() {
	if style == nil {
		style = styles.Fallback
	}
	if formatter == nil {
		formatter = formatters.Fallback
	}
	chromaReady = style != nil && formatter != nil
}

func lexerFor(lang string) chroma.Lexer {
	if v, ok := lexerCache.Load(lang); ok {
		return v.(chroma.Lexer)
	}
	l := lexers.Get(lang)
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)
	lexerCache.Store(lang, l)
	return l
}

// highlightLine returns a single line with ANSI color codes applied.
// Falls back to the plain line if chroma fails. Always strips trailing newline.
func highlightLine(lang, line string) string {
	if !chromaReady || line == "" {
		return line
	}
	l := lexerFor(lang)
	it, err := l.Tokenise(nil, line+"\n")
	if err != nil {
		return line
	}
	buf := bufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufPool.Put(buf)
	}()
	if err := formatter.Format(buf, style, it); err != nil {
		return line
	}
	return strings.TrimRight(buf.String(), "\n")
}
