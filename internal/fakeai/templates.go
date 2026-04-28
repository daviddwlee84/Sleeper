package fakeai

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// ChatStep is one role+body line of a conversation.
type ChatStep struct {
	Role string // "user" or "ai"
	Body string // already-rendered text (template substitutions applied)
}

// Conversation is a sequence of steps under a named style.
type Conversation struct {
	Style string
	Steps []ChatStep
}

type rawConv struct {
	style string
	steps []rawStep
}

type rawStep struct {
	role string
	body string
}

// LoadAll parses every embedded template into a flat slice of conversations.
// Returns at least one conversation or an error.
func LoadAll() ([]rawConv, error) {
	var out []rawConv
	err := fs.WalkDir(templatesFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(p, ".tmpl") {
			return nil
		}
		style := strings.TrimSuffix(d.Name(), ".tmpl")
		data, err := templatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		convs := splitConversations(style, data)
		out = append(out, convs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no conversations parsed")
	}
	return out, nil
}

// splitConversations splits a template file by `---` separators and parses
// each conversation into role|body pairs.
func splitConversations(style string, data []byte) []rawConv {
	var out []rawConv
	for _, chunk := range bytes.Split(data, []byte("\n---\n")) {
		lines := strings.Split(string(chunk), "\n")
		var steps []rawStep
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			i := strings.Index(ln, "|")
			if i <= 0 {
				continue
			}
			role := ln[:i]
			body := ln[i+1:]
			if role != "user" && role != "ai" {
				continue
			}
			steps = append(steps, rawStep{role: role, body: body})
		}
		if len(steps) > 0 {
			out = append(out, rawConv{style: style, steps: steps})
		}
	}
	return out
}

// Substitution is the data passed into the text/template renderer.
type Substitution struct {
	File string
	Func string
	Line int
}

// Render applies the substitution to a raw conversation, returning a Conversation
// whose ChatStep bodies have placeholders filled in.
func Render(rc rawConv, sub Substitution) (Conversation, error) {
	out := Conversation{Style: rc.style, Steps: make([]ChatStep, 0, len(rc.steps))}
	for _, s := range rc.steps {
		t, err := template.New("").Option("missingkey=zero").Parse(s.body)
		if err != nil {
			return Conversation{}, err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, sub); err != nil {
			return Conversation{}, err
		}
		out.Steps = append(out.Steps, ChatStep{Role: s.role, Body: buf.String()})
	}
	return out, nil
}
