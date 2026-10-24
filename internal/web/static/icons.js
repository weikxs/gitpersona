// Inline icons (outline style, 24px grid), drawn here so the page loads
// nothing from outside. Icons are decoration: their meaning is always in
// the text next to them.

const PATHS = {
  home: ["M3 10.5 12 3l9 7.5", "M5 9.5V20h5v-6h4v6h5V9.5"],
  user: ["M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z", "M4 20c1.5-3.6 4.4-5.5 8-5.5s6.5 1.9 8 5.5"],
  repo: ["M5 4.5A1.5 1.5 0 0 1 6.5 3H19v15H6.5A1.5 1.5 0 0 0 5 19.5v-15Z", "M5 19.5A1.5 1.5 0 0 0 6.5 21H19v-3", "M9 7h6"],
  check: ["M5 12.5 10 17l9-10"],
  diff: ["M8 3v12", "M4 7l4-4 4 4", "M16 21V9", "M12 17l4 4 4-4"],
  terminal: ["M3 5h18v14H3z", "M7 9l3 3-3 3", "M12 15h5"],
  clock: ["M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z", "M12 7v5l3 2"],
  folder: ["M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5v9a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5v-11Z"],
  pin: ["M9 3h6l-1 6 3 3H7l3-3-1-6Z", "M12 12v9"],
  alert: ["M12 3 2 20h20L12 3Z", "M12 10v4", "M12 17h.01"],
  shield: ["M12 3 4 6v6c0 4.5 3.4 8 8 9 4.6-1 8-4.5 8-9V6l-8-3Z", "M8.5 12l2.5 2.5 4.5-5"],
  info: ["M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z", "M12 11v5", "M12 8h.01"],
  wand: ["M4 20 15 9", "M15 4v2", "M20 9h-2", "M18.5 5.5 17 7", "M11 4l.5 1.5L13 6l-1.5.5L11 8l-.5-1.5L9 6l1.5-.5Z"],
};

const NS = "http://www.w3.org/2000/svg";

// icon returns an inline SVG for name, hidden from assistive technology.
export function icon(name) {
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "1.8");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("focusable", "false");
  for (const d of PATHS[name] ?? []) {
    const path = document.createElementNS(NS, "path");
    path.setAttribute("d", d);
    svg.append(path);
  }
  return svg;
}
