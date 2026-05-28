package render

import (
	"regexp"
	"strings"
)

// alertPattern matches the opening line of a GitHub alert blockquote:
// > [!NOTE], > [!WARNING], etc.
var alertPattern = regexp.MustCompile(`(?im)^((?:> [^\n]*\n)*> \[!(NOTE|WARNING|TIP|IMPORTANT|CAUTION)\]\n((?:> [^\n]*\n?)*))`)

// preprocessAlerts transforms GitHub alert syntax in markdown source into
// raw HTML div blocks that goldmark passes through as-is (requires WithUnsafe).
// Input: []byte markdown; output: []byte with alerts replaced.
func preprocessAlerts(src []byte) []byte {
	return alertPattern.ReplaceAllFunc(src, func(match []byte) []byte {
		return convertAlertBlock(string(match))
	})
}

// alertBlockPattern matches a complete blockquote alert block:
// first line is the marker, remaining lines are body.
var alertBlockPattern = regexp.MustCompile(`(?i)^> \[!(NOTE|WARNING|TIP|IMPORTANT|CAUTION)\]\n((?:> [^\n]*\n?)*)`)

func convertAlertBlock(block string) []byte {
	m := alertBlockPattern.FindStringSubmatch(block)
	if m == nil {
		return []byte(block)
	}
	keyword := strings.ToUpper(m[1])
	lower := strings.ToLower(keyword)
	title := strings.ToUpper(keyword[:1]) + strings.ToLower(keyword[1:])

	// strip "> " prefix from body lines
	rawLines := strings.Split(strings.TrimRight(m[2], "\n"), "\n")
	var bodyLines []string
	for _, l := range rawLines {
		after, found := strings.CutPrefix(l, "> ")
		if found {
			bodyLines = append(bodyLines, after)
		} else if l == ">" {
			bodyLines = append(bodyLines, "")
		}
	}
	body := strings.Join(bodyLines, "\n")
	if body != "" {
		body = "<p>" + body + "</p>\n"
	}

	return []byte("<div class=\"markdown-alert markdown-alert-" + lower + "\"><p class=\"markdown-alert-title\">" + title + "</p>\n" + body + "</div>\n\n")
}
