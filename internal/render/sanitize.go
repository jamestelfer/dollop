package render

import "github.com/microcosm-cc/bluemonday"

// githubPolicy is a bluemonday policy that mirrors GitHub's HTML allowlist,
// permitting block and inline elements that GitHub renders but blocking
// unsafe tags like <script> and <style>.
var githubPolicy = buildGitHubPolicy()

func buildGitHubPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// structural / block
	p.AllowElements(
		"article", "aside", "details", "summary",
		"div", "section", "header", "footer", "main",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "blockquote", "pre", "ul", "ol", "li", "dl", "dt", "dd",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td",
		"br", "hr",
		"figure", "figcaption",
	)

	// inline / phrasing
	p.AllowElements(
		"a", "abbr", "b", "bdo", "cite", "code",
		"del", "dfn", "em", "i", "ins", "kbd",
		"mark", "q", "rp", "rt", "ruby",
		"s", "samp", "small", "span", "strong",
		"sub", "sup", "time", "tt", "u", "var", "wbr",
		"picture", "img",
	)

	// attributes
	p.AllowAttrs("id", "class", "aria-hidden", "aria-label").Globally()
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt", "width", "height", "loading").OnElements("img")
	p.AllowAttrs("src").OnElements("source")
	p.AllowAttrs("type", "checked", "disabled").OnElements("input")
	p.AllowAttrs("open").OnElements("details")
	p.AllowAttrs("align", "valign", "colspan", "rowspan").OnElements("table", "thead", "tbody", "tfoot", "tr", "th", "td")
	p.AllowAttrs("start", "type").OnElements("ol")
	p.AllowAttrs("datetime").OnElements("time", "del", "ins")
	p.AllowAttrs("data-*").Globally()

	p.AllowRelativeURLs(true)
	p.AllowURLSchemes("http", "https", "mailto", "ftp")

	return p
}

func sanitizeHTML(body string) string {
	return githubPolicy.Sanitize(body)
}
