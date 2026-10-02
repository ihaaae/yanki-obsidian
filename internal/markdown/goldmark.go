// Package markdown converts a Markdown document into a Yanki note, inferring
// the Anki note type from the document's structure.
package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// Goldmark parsers are safe for concurrent use, so two shared instances cover
// both line-break settings.
//
// GFM provides tables, strikethrough (which Yanki reads as cloze deletions),
// task lists, and autolinks. Unsafe HTML rendering keeps hand-written HTML in
// notes intact, matching the source Markdown rather than escaping it.
var (
	parser = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	// parserHardWraps renders single newlines as <br>, matching Obsidian's
	// default "Strict line breaks" setting of off.
	parserHardWraps = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(html.WithUnsafe(), html.WithHardWraps()),
	)
)

// selectParser picks the parser for a line-break setting.
func selectParser(strictLineBreaks bool) goldmark.Markdown {
	if strictLineBreaks {
		return parser
	}

	return parserHardWraps
}
