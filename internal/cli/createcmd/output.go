package createcmd

import (
	"io"

	"golang.org/x/term"
)

// hyperlink wraps text in an OSC 8 terminal hyperlink escape sequence.
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// formatURL returns url as a plain string for non-TTY output, or wrapped in
// an OSC 8 hyperlink escape sequence when isTTY is true.
func formatURL(url string, isTTY bool) string {
	if isTTY {
		return hyperlink(url, url)
	}
	return url
}

// isTerminalWriter reports whether w is a file descriptor connected to a terminal.
func isTerminalWriter(w io.Writer) bool {
	type fder interface{ Fd() uintptr }
	if f, ok := w.(fder); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
