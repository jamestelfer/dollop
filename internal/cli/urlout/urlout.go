// Package urlout formats published upload URLs for terminal output, emitting
// OSC 8 hyperlinks on a TTY and a plain URL otherwise.
package urlout

import (
	"io"

	"golang.org/x/term"
)

// hyperlink wraps text in an OSC 8 terminal hyperlink escape sequence.
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// Format returns url as a plain string for non-TTY output, or wrapped in an
// OSC 8 hyperlink escape sequence when isTTY is true.
func Format(url string, isTTY bool) string {
	if isTTY {
		return hyperlink(url, url)
	}
	return url
}

// IsTerminalWriter reports whether w is a file descriptor connected to a terminal.
func IsTerminalWriter(w io.Writer) bool {
	type fder interface{ Fd() uintptr }
	if f, ok := w.(fder); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
