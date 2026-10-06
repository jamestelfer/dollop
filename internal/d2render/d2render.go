// Package d2render renders d2 diagram source to SVG. It is the only package
// that imports the d2 library.
package d2render

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/d2/d2layouts/d2talalayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
)

const defaultEngine = "tala"

// defaultPad is the padding, in pixels, around the diagram inside the SVG.
// d2's own default (100) leaves wide margins; the page already spaces figures.
const defaultPad int64 = 16

// Diagram is the rendered form of one d2 source.
type Diagram struct {
	SVG           []byte
	Width, Height int // intrinsic pixel dimensions, taken from the root viewBox
}

// d2 emits the root viewBox with a zero origin and integer dimensions.
var viewBoxRE = regexp.MustCompile(`viewBox="0 0 (\d+) (\d+)"`)

// Render compiles src and renders it to a single SVG that embeds both the light
// and the dark theme, switched by the SVG's own prefers-color-scheme query.
func Render(ctx context.Context, src []byte) (Diagram, error) {
	input := string(src)

	engine, pad, err := sourceConfig(input)
	if err != nil {
		return Diagram{}, err
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return Diagram{}, fmt.Errorf("create text ruler: %w", err)
	}

	compileOpts := &d2lib.CompileOptions{
		Ruler:          ruler,
		Layout:         &engine,
		LayoutResolver: resolveLayout,
		RouterResolver: resolveRouter,
	}
	renderOpts := &d2svg.RenderOpts{
		ThemeID:     &d2themescatalog.NeutralDefault.ID,
		DarkThemeID: &d2themescatalog.DarkMauve.ID,
		Pad:         &pad,
	}

	diagram, _, err := d2lib.Compile(log.WithDefault(ctx), input, compileOpts, renderOpts)
	if err != nil {
		return Diagram{}, err
	}

	svg, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return Diagram{}, fmt.Errorf("render svg: %w", err)
	}

	m := viewBoxRE.FindSubmatch(svg)
	if m == nil {
		return Diagram{}, errors.New("rendered svg has no root viewBox")
	}
	width, _ := strconv.Atoi(string(m[1]))
	height, _ := strconv.Atoi(string(m[2]))

	return Diagram{SVG: svg, Width: width, Height: height}, nil
}

// sourceConfig returns the layout engine and padding named by the source's
// d2-config, falling back to defaultEngine and defaultPad. d2lib cannot express
// these defaults: options passed to it override the source, and with none
// passed it falls back to its own (dagre, 100px padding).
func sourceConfig(input string) (engine string, pad int64, err error) {
	_, config, err := d2compiler.Compile("", strings.NewReader(input), nil)
	if err != nil {
		return "", 0, err
	}
	engine, pad = defaultEngine, defaultPad
	if config != nil && config.LayoutEngine != nil {
		engine = *config.LayoutEngine
	}
	if config != nil && config.Pad != nil {
		pad = *config.Pad
	}
	return engine, pad, nil
}

func resolveLayout(engine string) (d2graph.LayoutGraph, error) {
	switch engine {
	case "dagre":
		return d2dagrelayout.DefaultLayout, nil
	case "elk":
		return d2elklayout.DefaultLayout, nil
	case "tala":
		return d2talalayout.DefaultLayout, nil
	default:
		return nil, fmt.Errorf("unsupported layout engine %q", engine)
	}
}

// TALA routes its own edges; the other engines use d2's default router.
func resolveRouter(engine string) (d2graph.RouteEdges, error) {
	if engine == "tala" {
		return d2talalayout.RouteEdges, nil
	}
	return d2layouts.DefaultRouter, nil
}
