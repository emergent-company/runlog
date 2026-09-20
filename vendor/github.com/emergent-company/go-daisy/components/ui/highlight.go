package ui

import (
	"bytes"
	"html"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var highlightStyle = styles.Get("github-dark")

// SetHighlightStyle sets the chroma style used for all highlighting.
// Default: "github-dark". Call once at app startup.
func SetHighlightStyle(name string) {
	if s := styles.Get(name); s != nil {
		highlightStyle = s
	}
}

// HighlightHTML applies server-side syntax highlighting to the given source
// code using chroma. lang is the language identifier (e.g. "go", "html").
// Returns inline-styled HTML suitable for passing to CodeBlock.Highlight.
// Panics on unrecoverable lexer/formatter errors (should never happen in practice).
func Highlight(src, lang string) string {
	l := lexers.Get(lang)
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)

	it, err := l.Tokenise(nil, src)
	if err != nil {
		return html.EscapeString(src)
	}

	formatter := chromahtml.New(chromahtml.WithClasses(false), chromahtml.TabWidth(2), chromahtml.Standalone(false))
	var buf bytes.Buffer
	if err := formatter.Format(&buf, highlightStyle, it); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
}
