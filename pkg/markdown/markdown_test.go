package markdown_test

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/markdown"
)

func TestToHTML_Empty(t *testing.T) {
	if got := markdown.ToHTML(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestToHTML_PlainText(t *testing.T) {
	got := markdown.ToHTML("Hello world")
	want := "<p>Hello world</p>\n"
	if got != want {
		t.Fatalf("plain text: want %q, got %q", want, got)
	}
}

func TestToHTML_Heading(t *testing.T) {
	got := markdown.ToHTML("# Heading")
	if !strings.Contains(got, "<h1") || !strings.Contains(got, "Heading</h1>") {
		t.Fatalf("expected <h1>Heading</h1>, got %q", got)
	}
}

func TestToHTML_Bold(t *testing.T) {
	got := markdown.ToHTML("This is **bold** text.")
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("expected <strong> wrapping, got %q", got)
	}
}

func TestToHTML_Italic(t *testing.T) {
	got := markdown.ToHTML("This is *italic* text.")
	if !strings.Contains(got, "<em>italic</em>") {
		t.Fatalf("expected <em> wrapping, got %q", got)
	}
}

func TestToHTML_Link(t *testing.T) {
	got := markdown.ToHTML("See [docs](https://example.com).")
	want := `<a href="https://example.com">docs</a>`
	if !strings.Contains(got, want) {
		t.Fatalf("expected anchor %q in output, got %q", want, got)
	}
}

func TestToHTML_BulletList(t *testing.T) {
	got := markdown.ToHTML("- one\n- two\n- three\n")
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<li>one</li>") {
		t.Fatalf("expected <ul><li>one</li>... in output, got %q", got)
	}
}

func TestToHTML_NumberedList(t *testing.T) {
	got := markdown.ToHTML("1. first\n2. second\n")
	if !strings.Contains(got, "<ol>") || !strings.Contains(got, "<li>first</li>") {
		t.Fatalf("expected <ol><li>first</li> in output, got %q", got)
	}
}

func TestToHTML_InlineCode(t *testing.T) {
	got := markdown.ToHTML("Use `Ctrl+C` to copy.")
	if !strings.Contains(got, "<code>Ctrl+C</code>") {
		t.Fatalf("expected <code>Ctrl+C</code>, got %q", got)
	}
}

func TestToHTML_PassesThroughExistingHTML(t *testing.T) {
	in := `<p>Already <strong>HTML</strong>.</p>`
	got := markdown.ToHTML(in)
	if !strings.Contains(got, "<p>Already <strong>HTML</strong>.</p>") {
		t.Fatalf("expected existing HTML preserved, got %q", got)
	}
}

func TestToHTML_GFMTable(t *testing.T) {
	in := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	got := markdown.ToHTML(in)
	if !strings.Contains(got, "<table>") || !strings.Contains(got, "<td>1</td>") {
		t.Fatalf("expected GFM table rendering, got %q", got)
	}
}

func TestIsRichTextStringType(t *testing.T) {
	cases := map[string]bool{
		"RICH_TEXT":  true,
		"PLAIN_TEXT": false,
		"":           false,
		"rich_text":  false,
	}
	for in, want := range cases {
		if got := markdown.IsRichTextStringType(in); got != want {
			t.Errorf("IsRichTextStringType(%q): want %v, got %v", in, want, got)
		}
	}
}
