/*
 * Click to zoom on diagrams. On wide screens a click on a d2 or mermaid
 * diagram shows it in a modal dialog, scaled to fit the viewport but at most
 * twice its natural size; a click anywhere or Escape closes it. Narrow
 * screens, and modified clicks on d2 diagrams, keep the link to the
 * standalone SVG, which pinch-zoom handles.
 *
 * Loaded async: it only registers listeners, and touches the DOM on click.
 */
(function () {
  "use strict";

  var MAX_SCALE = 2;
  var wide = window.matchMedia("(min-width: 50rem)");
  var dialog = null;

  // The diagram's natural size: a d2 image's width/height attributes, or a
  // mermaid SVG's viewBox.
  function naturalSize(diagram) {
    var w = Number(diagram.getAttribute("width"));
    var h = Number(diagram.getAttribute("height"));
    var box = diagram.viewBox && diagram.viewBox.baseVal;
    if (diagram.tagName !== "IMG" && box && box.width) {
      w = box.width;
      h = box.height;
    }
    if (!w || !h) {
      var r = diagram.getBoundingClientRect();
      w = r.width;
      h = r.height;
    }
    return [w, h];
  }

  function open(diagram) {
    if (!dialog) {
      dialog = document.createElement("dialog");
      dialog.className = "diagram-zoom";
      dialog.setAttribute("aria-label", "Diagram");
      dialog.addEventListener("click", function () { dialog.close(); });
      dialog.addEventListener("close", function () { dialog.replaceChildren(); });
      document.body.appendChild(dialog);
    }
    var size = naturalSize(diagram);
    // Room left by the dialog's 2rem margin, 1.5rem padding and 1px border.
    var rem = parseFloat(getComputedStyle(document.documentElement).fontSize);
    var room = 7 * rem + 2;
    var scale = Math.min(MAX_SCALE,
      (window.innerWidth - room) / size[0],
      (window.innerHeight - room) / size[1]);
    var clone = diagram.cloneNode(true);
    clone.style.width = size[0] * scale + "px";
    clone.style.height = size[1] * scale + "px";
    dialog.replaceChildren(clone);
    dialog.showModal();
  }

  document.addEventListener("click", function (ev) {
    if (!wide.matches || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
    var target = ev.target.closest ? ev.target.closest(".markdown-body a.d2, .markdown-body pre.mermaid") : null;
    if (!target) return;
    // A mermaid block is only zoomable once mermaid has replaced its source.
    var diagram = target.querySelector(target.tagName === "A" ? "img" : "svg");
    if (!diagram) return;
    ev.preventDefault();
    open(diagram);
  });
})();
