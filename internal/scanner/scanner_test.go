package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot finds the project root by walking up to the directory containing go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			t.Fatal("go.mod not found")
		}
		d = parent
	}
}

func TestNew_Dogfood(t *testing.T) {
	s, err := New(repoRoot(t), 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(s.Files()) == 0 {
		t.Fatal("expected at least one file from this repo")
	}
	for _, f := range s.Files() {
		if strings.Contains(f.Rel, ".git/") || strings.HasPrefix(f.Rel, ".git/") {
			t.Errorf(".git path leaked: %s", f.Rel)
		}
	}
}

func TestPrivacyDeny(t *testing.T) {
	dir := t.TempDir()
	must := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must(".env", "SECRET=abc")
	must(".env.production", "API=xyz")
	must("id_rsa", "-----BEGIN PRIVATE KEY-----")
	must("server.pem", "-----BEGIN CERT-----")
	must("safe.go", "package main\nfunc main(){}\n")

	s, err := New(dir, 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := len(s.Files()); got != 1 {
		t.Fatalf("expected 1 safe file, got %d: %+v", got, s.Files())
	}
	if !strings.HasSuffix(s.Files()[0].Rel, "safe.go") {
		t.Errorf("expected safe.go, got %s", s.Files()[0].Rel)
	}
}

func TestBinaryRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blob.go"), []byte{0, 1, 2, 3, 0, 0, 0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(dir, 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, f := range s.Files() {
		if strings.HasSuffix(f.Rel, "blob.go") {
			t.Errorf("binary blob.go should have been rejected")
		}
	}
}

func TestSizeLimit(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("// hello\n", 10000) // ~90KB > 64KB
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(dir, 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, f := range s.Files() {
		if strings.HasSuffix(f.Rel, "big.go") {
			t.Errorf("big.go should have been rejected by size limit")
		}
	}
}

func TestSymbols_Go(t *testing.T) {
	dir := t.TempDir()
	src := "package x\n\nfunc Alpha() {}\nfunc (r *R) Beta() {}\nfunc gamma() {}\n"
	path := filepath.Join(dir, "x.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(dir, 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	syms := s.Symbols(s.Files()[0], 10)
	want := map[string]bool{"Alpha": true, "Beta": true, "gamma": true}
	for _, sym := range syms {
		delete(want, sym)
	}
	if len(want) != 0 {
		t.Errorf("missing symbols: %v (got %v)", want, syms)
	}
}

func TestPickDeterministic(t *testing.T) {
	root := repoRoot(t)
	a, _ := New(root, 42)
	b, _ := New(root, 42)
	for i := 0; i < 5; i++ {
		if a.Pick().Rel != b.Pick().Rel {
			t.Fatal("same seed should produce same picks")
		}
	}
}
