package render_test

import (
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
)

func TestIsAssetName(t *testing.T) {
	for name, want := range map[string]bool{
		".dollop/github-markdown.css":      true,
		".dollop/d2/0123456789ab.svg":      true,
		".dollop/anything.txt":             true, // the asset directory belongs to dollop
		".dollop/":                         false,
		"docs/.dollop/github-markdown.css": false, // the asset directory is at the prefix root
		// assets older releases published beside the pages
		"github-markdown.css":      true,
		"dollop-markdown.css":      true,
		"highlight-github.css":     true,
		"dollop-favicon.svg":       true,
		"dollop-theme-tide.css":    true,
		"dollop-theme-retired.css": true, // a theme from an older release
		"d2/0123456789ab.svg":      true,
		"d2/notes.svg":             false,
		"docs/github-markdown.css": false, // assets live at the prefix root
		"dollop-theme-.css":        false,
		"styles.css":               false,
		"notes.html":               false,
	} {
		assert.Equal(t, want, render.IsAssetName(name), name)
	}
}

func TestInAssetDir(t *testing.T) {
	for name, want := range map[string]bool{
		".dollop/dollop-dark.svg":     true,
		".dollop/d2/0123456789ab.svg": true,
		".dollop/":                    false,
		"docs/.dollop/x.svg":          false,
		"github-markdown.css":         false, // a legacy asset name is not in the asset directory
	} {
		assert.Equal(t, want, render.InAssetDir(name), name)
	}
}
