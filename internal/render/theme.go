package render

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed assets/dollop-theme-tide.css
var tideThemeCSS []byte

//go:embed assets/dollop-theme-estuary.css
var estuaryThemeCSS []byte

//go:embed assets/dollop-theme-lichen.css
var lichenThemeCSS []byte

// plexFonts loads Bricolage Grotesque, IBM Plex Sans and IBM Plex Mono in the
// weights the Plex-based themes use.
const plexFonts = "https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,500;12..96,650" +
	"&family=IBM+Plex+Sans:ital,wght@0,400;0,500;0,600;1,400;1,600" +
	"&family=IBM+Plex+Mono:wght@400;500&display=swap"

// DefaultTheme is the theme used when none is chosen.
const DefaultTheme = "tide"

// Theme selects how rendered pages look. Each theme is a token stylesheet that
// defines the semantic font and colour tokens dollop-markdown.css reads, plus
// the web fonts those tokens name.
type Theme struct {
	Name        string
	Description string
	css         SharedAsset
	// sharedFont declares the optional PP Mori faces from deps/fonts/ and
	// loads Inter from Google Fonts only when they are unavailable.
	sharedFont bool
	// fontStylesheet is a web-font stylesheet URL the page links directly, or
	// empty when the theme needs none.
	fontStylesheet string
}

var themes = []Theme{
	{
		Name:        "tide",
		Description: "PP Mori (or Inter) on GitHub's neutral palette with the dollop teal accent",
		css:         themeAsset("tide", tideThemeCSS),
		sharedFont:  true,
	},
	{
		Name:           "estuary",
		Description:    "tide's colours with lichen's Bricolage Grotesque and IBM Plex type and rhythm",
		css:            themeAsset("estuary", estuaryThemeCSS),
		fontStylesheet: plexFonts,
	},
	{
		Name:           "lichen",
		Description:    "Bricolage Grotesque headings and IBM Plex text on a soft green-grey ground",
		css:            themeAsset("lichen", lichenThemeCSS),
		fontStylesheet: plexFonts,
	},
}

func themeAsset(name string, css []byte) SharedAsset {
	return SharedAsset{Name: "dollop-theme-" + name + ".css", ContentType: "text/css; charset=utf-8", Content: css}
}

// ThemeNames returns the names of the available themes, default first.
func ThemeNames() []string {
	names := make([]string, len(themes))
	for i, t := range themes {
		names[i] = t.Name
	}
	return names
}

// LookupTheme returns the theme with the given name. An empty name selects
// DefaultTheme.
func LookupTheme(name string) (Theme, error) {
	if name == "" {
		name = DefaultTheme
	}
	for _, t := range themes {
		if t.Name == name {
			return t, nil
		}
	}
	return Theme{}, fmt.Errorf("unknown theme %q (available: %s)", name, strings.Join(ThemeNames(), ", "))
}

func defaultTheme() Theme {
	t, err := LookupTheme(DefaultTheme)
	if err != nil {
		panic(err)
	}
	return t
}
