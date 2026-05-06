// Package markdown converts agent-emitted Markdown to HTML for Collibra
// rich-text attribute fields. Collibra renders RICH_TEXT attributes (e.g.
// "Definition") as HTML, so Markdown syntax in an LLM's output otherwise
// displays as raw characters in the UI (DEV-179098). The intended caller
// is a write tool that has already resolved an attribute's stringType to
// "RICH_TEXT" — plain-string attributes should bypass this package.
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// converter is configured once at init: GFM (tables, strikethrough,
// task-list, autolinks) covers what LLMs typically emit, and WithUnsafe
// preserves any HTML the agent already emitted instead of escaping it.
// chip is not the security boundary here — Collibra's UI is responsible
// for safe rendering of its own attribute values.
var converter = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// ToHTML renders s, treated as Markdown, to HTML. An empty input returns
// an empty string. If goldmark fails (only possible on a writer error,
// which a bytes.Buffer cannot produce), the original string is returned
// unchanged so the agent's data is never silently dropped.
//
// Plain text without any Markdown syntax round-trips as a paragraph-wrapped
// string (e.g. "Hello" → "<p>Hello</p>"); Collibra's UI renders this
// identically to the bare text, which satisfies the "plain text passes
// through unaffected" criterion in DEV-179098.
func ToHTML(s string) string {
	if s == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := converter.Convert([]byte(s), &buf); err != nil {
		return s
	}
	return buf.String()
}

// IsRichTextStringType reports whether a Collibra attribute type's
// stringType field signals that the attribute stores HTML. Used by write
// tools to decide whether to run a value through ToHTML before submitting.
func IsRichTextStringType(stringType string) bool {
	return stringType == "RICH_TEXT"
}
