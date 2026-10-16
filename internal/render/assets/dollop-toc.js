/*
 * "On this page" track, adapted from the Fumadocs clerk table of contents
 * (MIT, https://github.com/fuma-nama/fumadocs).
 *
 * A faint line steps in and out with heading depth beside the links. The same
 * line is drawn again in the accent colour and clipped to the span of the
 * sections in view; the CSS transition on that clip animates it as the reader
 * scrolls. A section runs from its heading to the next, so the thumb stays put
 * while reading a long section with no heading on screen.
 */
(function () {
  "use strict";

  var nav = document.querySelector(".page-toc");
  if (!nav) return;
  var column = nav.parentElement;
  var track = nav.querySelector(".page-toc-track");
  var header = document.querySelector(".page-header");
  var links = [];
  var headings = [];
  Array.prototype.forEach.call(nav.querySelectorAll(".page-toc-link"), function (a) {
    var h = document.getElementById(decodeURIComponent(a.hash.slice(1)));
    if (h) {
      links.push(a);
      headings.push(h);
    }
  });
  if (links.length === 0) return;

  var SVG_NS = "http://www.w3.org/2000/svg";
  function line(cls) {
    var svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("class", "page-toc-line " + cls);
    svg.setAttribute("aria-hidden", "true");
    var path = document.createElementNS(SVG_NS, "path");
    path.setAttribute("fill", "none");
    path.setAttribute("stroke-width", "1");
    svg.appendChild(path);
    track.insertBefore(svg, track.firstChild);
    return svg;
  }
  // Each line is inserted first, so the thumb, created first, paints over the
  // base.
  var thumb = line("page-toc-line-thumb");
  var base = line("page-toc-line-base");

  // Line x offset per heading depth; the links are indented 12px past it.
  function lineOffset(depth) {
    if (depth <= 2) return 8;
    if (depth === 3) return 20;
    return 32;
  }

  // positions[i] is [top, bottom] of link i's text within the track.
  var positions = [];

  function draw() {
    if (track.clientHeight === 0) return; // hidden on narrow viewports
    var d = "";
    var w = 0;
    var h = 0;
    var prev = null;
    positions = [];
    links.forEach(function (a) {
      var style = getComputedStyle(a);
      var x = lineOffset(Number(a.dataset.depth)) + 0.5;
      var top = a.offsetTop + parseFloat(style.paddingTop);
      var bottom = a.offsetTop + a.clientHeight - parseFloat(style.paddingBottom);
      d += prev ? " L" + prev[0] + " " + prev[1] + " " + x + " " + top : "M" + x + " " + top;
      d += " L" + x + " " + bottom;
      prev = [x, bottom];
      w = Math.max(w, x + 8);
      h = Math.max(h, bottom);
      positions.push([top, bottom]);
    });
    [base, thumb].forEach(function (svg) {
      svg.setAttribute("width", w);
      svg.setAttribute("height", h);
      svg.setAttribute("viewBox", "0 0 " + w + " " + h);
      svg.firstChild.setAttribute("d", d);
    });
  }

  function update() {
    if (positions.length === 0) return;
    // Fragment jumps leave a sliver of the previous section between the header
    // and the heading (scroll-padding-top); the margin keeps it inactive.
    var viewTop = (header ? header.getBoundingClientRect().bottom : 0) + 24;
    var viewBottom = window.innerHeight;
    var first = -1;
    var last = -1;
    var tops = headings.map(function (el) { return el.getBoundingClientRect().top; });
    for (var i = 0; i < tops.length; i++) {
      var end = i + 1 < tops.length ? tops[i + 1] : Infinity;
      if (tops[i] < viewBottom && end > viewTop) {
        if (first < 0) first = i;
        last = i;
      }
    }
    links.forEach(function (a, j) {
      a.classList.toggle("active", first >= 0 && j >= first && j <= last);
    });
    var trackTop = first >= 0 ? positions[first][0] : 0;
    var trackBottom = first >= 0 ? positions[last][1] : 0;
    track.style.setProperty("--track-top", trackTop + "px");
    track.style.setProperty("--track-bottom", trackBottom + "px");
    if (first >= 0) reveal(links[first]);
  }

  // Keep the first active link in view when the column scrolls.
  function reveal(a) {
    var r = a.getBoundingClientRect();
    var c = column.getBoundingClientRect();
    if (r.top < c.top || r.bottom > c.bottom) {
      column.scrollTop += r.top - c.top - c.height / 3;
    }
  }

  var pending = false;
  function schedule(redraw) {
    if (redraw) positions = [];
    if (pending) return;
    pending = true;
    requestAnimationFrame(function () {
      pending = false;
      if (positions.length === 0) draw();
      update();
    });
  }

  draw();
  update();
  // Enable the transition only after the first placement, so the thumb does
  // not sweep down from the top on load.
  requestAnimationFrame(function () { track.classList.add("ready"); });

  window.addEventListener("scroll", function () { schedule(false); }, { passive: true });
  window.addEventListener("resize", function () { schedule(true); });
  if (window.ResizeObserver) {
    var resized = new ResizeObserver(function () { schedule(true); });
    resized.observe(track);
    resized.observe(document.body);
  }
})();
