package scanner

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// File describes a file from the target project that is safe to display.
type File struct {
	Path string // absolute path
	Rel  string // relative to project root
	Lang string // chroma lexer hint (go, python, ...)
	Size int64
}

type Scanner struct {
	Root  string
	files []File
	rng   *rand.Rand
}

const maxFileSize int64 = 200 * 1024

// fnames or globs we never want to fake-display because they often hold secrets.
var privacyDeny = []string{
	".env", ".env.*", "*.pem", "*.key",
	"id_rsa", "id_rsa.*", "id_ed25519", "id_ed25519.*",
	"*.p12", "*.pfx", "credentials*", "*.kdbx",
}

// directories we never recurse into when not using `git ls-files`.
var ignoreDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, ".venv": true, "venv": true, "__pycache__": true,
	".next": true, ".cache": true, "target": true, ".idea": true, ".vscode": true,
}

// extension weights bias the picker toward "real code" files.
var extWeight = map[string]int{
	".go": 10, ".py": 10, ".ts": 10, ".tsx": 9, ".js": 8, ".jsx": 7,
	".rs": 9, ".java": 7, ".kt": 7, ".swift": 7, ".rb": 7, ".php": 6,
	".c": 8, ".cc": 8, ".cpp": 8, ".h": 6, ".hpp": 6,
	".sh": 4, ".sql": 5,
	".yaml": 2, ".yml": 2, ".toml": 2, ".json": 1,
	".md": 1, ".txt": 1,
}

var langForExt = map[string]string{
	".go": "go", ".py": "python", ".ts": "typescript", ".tsx": "tsx",
	".js": "javascript", ".jsx": "jsx", ".rs": "rust", ".java": "java",
	".kt": "kotlin", ".swift": "swift", ".rb": "ruby", ".php": "php",
	".c": "c", ".cc": "cpp", ".cpp": "cpp", ".h": "c", ".hpp": "cpp",
	".sh": "bash", ".sql": "sql",
	".yaml": "yaml", ".yml": "yaml", ".toml": "toml", ".json": "json",
	".md": "markdown",
}

// New scans the given project root and returns a Scanner ready to Pick().
func New(root string, seed uint64) (*Scanner, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}
	var rels []string
	if isGitRepo(abs) {
		rels, err = gitList(abs)
		if err != nil {
			return nil, err
		}
	} else {
		rels, err = walkList(abs)
		if err != nil {
			return nil, err
		}
	}
	files := filterFiles(abs, rels)
	if len(files) == 0 {
		return nil, fmt.Errorf("no usable source files under %s", abs)
	}
	if seed == 0 {
		seed = randomSeed()
	}
	return &Scanner{
		Root:  abs,
		files: files,
		rng:   rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
	}, nil
}

func randomSeed() uint64 {
	var b [8]byte
	_, _ = os.ReadFile("/dev/urandom") // best effort, unused
	for i := range b {
		b[i] = byte(os.Getpid() >> uint(i))
	}
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
}

// Files returns a snapshot of the discovered files.
func (s *Scanner) Files() []File { return append([]File(nil), s.files...) }

// Pick returns a random file weighted by extension importance.
func (s *Scanner) Pick() File {
	total := 0
	weights := make([]int, len(s.files))
	for i, f := range s.files {
		w := extWeight[strings.ToLower(filepath.Ext(f.Path))]
		if w == 0 {
			w = 1
		}
		weights[i] = w
		total += w
	}
	r := s.rng.IntN(total)
	for i, w := range weights {
		if r < w {
			return s.files[i]
		}
		r -= w
	}
	return s.files[len(s.files)-1]
}

// symbol regexes per language. Conservative, just for visual flavor in AI templates.
var symbolRegexes = map[string]*regexp.Regexp{
	"go":         regexp.MustCompile(`(?m)^func(?:\s+\([^)]+\))?\s+([A-Za-z_][A-Za-z0-9_]*)`),
	"python":     regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)`),
	"typescript": regexp.MustCompile(`(?m)\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)`),
	"javascript": regexp.MustCompile(`(?m)\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)`),
	"rust":       regexp.MustCompile(`(?m)^\s*fn\s+([A-Za-z_][A-Za-z0-9_]*)`),
}

// Symbols extracts up to N likely function names from a file.
func (s *Scanner) Symbols(f File, max int) []string {
	re := symbolRegexes[f.Lang]
	if re == nil {
		return nil
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil
	}
	matches := re.FindAllSubmatch(data, max)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			out = append(out, string(m[1]))
		}
	}
	return out
}

func isGitRepo(root string) bool {
	st, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil && (st.IsDir() || st.Mode().IsRegular())
}

func gitList(root string) ([]string, error) {
	// -c: cached (tracked), -o: others (untracked), --exclude-standard: respect .gitignore
	cmd := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var rels []string
	for _, b := range bytes.Split(buf.Bytes(), []byte{0}) {
		if len(b) == 0 {
			continue
		}
		rels = append(rels, string(b))
	}
	return rels, nil
}

func walkList(root string) ([]string, error) {
	var rels []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if ignoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rels = append(rels, rel)
		return nil
	})
	return rels, err
}

func filterFiles(root string, rels []string) []File {
	out := make([]File, 0, len(rels))
	for _, rel := range rels {
		base := filepath.Base(rel)
		if denyName(base) {
			continue
		}
		full := filepath.Join(root, rel)
		st, err := os.Stat(full)
		if err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > maxFileSize {
			continue
		}
		ext := strings.ToLower(filepath.Ext(rel))
		lang, ok := langForExt[ext]
		if !ok {
			continue // skip unknown extensions
		}
		if !looksTextual(full) {
			continue
		}
		out = append(out, File{Path: full, Rel: rel, Lang: lang, Size: st.Size()})
	}
	return out
}

func denyName(name string) bool {
	lower := strings.ToLower(name)
	for _, pat := range privacyDeny {
		if matched, _ := filepath.Match(pat, lower); matched {
			return true
		}
	}
	return false
}

func looksTextual(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return false
	}
	if bytes.IndexByte(buf[:n], 0) >= 0 {
		return false
	}
	ct := http.DetectContentType(buf[:n])
	return strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml") || strings.Contains(ct, "javascript")
}

// ReadLines returns the file content split by newline. Trims trailing newline only.
func ReadLines(f File) ([]string, error) {
	fh, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var lines []string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
