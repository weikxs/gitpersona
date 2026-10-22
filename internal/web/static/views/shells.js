// Shells: which shell is the login shell and which one runs now, and whether
// each installed shell loads GitPersona (for the gh account and prompt).

import { api } from "../api.js";
import { h, badge, busy } from "../dom.js";
import { factList } from "./overview.js";

function describe(shell) {
  if (!shell?.kind) return "Not detected";
  return [shell.kind, shell.path, shell.source ? `(${shell.source})` : ""].filter(Boolean).join(" ");
}

export function renderShells(section, { state, run }) {
  const shells = state.shells;
  section.append(
    h("div", { class: "card" },
      h("h3", {}, "Detected"),
      factList([["Login shell", describe(shells.default)], ["Running this page", describe(shells.current)]])),
    installedCard(shells.installed, run));
}

function installedCard(installed, run) {
  if (installed.length === 0) {
    return h("div", { class: "card" }, h("h3", {}, "Installed shells"),
      h("p", { class: "empty" }, "No supported shell (fish, zsh or bash) was found."));
  }
  const missing = installed.filter((s) => !s.integrated);
  const boxes = missing.map((s) => h("input", { type: "checkbox", name: "shell", value: s.kind, checked: true }));
  const submit = h("button", { type: "submit", class: "primary" }, "Add to selected shells");
  const form = h("form", { class: "stack" },
    h("fieldset", {},
      h("legend", {}, "Load GitPersona in"),
      missing.map((s, i) => h("label", { class: "check" }, boxes[i], `${s.kind} (${s.path})`))),
    h("div", { class: "actions" }, submit));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const chosen = boxes.filter((b) => b.checked).map((b) => b.value);
    if (chosen.length === 0) return;
    busy(submit, () => run(() => api.installShells(chosen),
      `Added GitPersona to ${chosen.join(", ")}. Open a new terminal to use it.`));
  });
  return h("div", { class: "card" },
    h("h3", {}, "Installed shells"),
    h("ul", { class: "rows" }, installed.map((s) =>
      h("li", {}, h("span", {}, s.kind, " ", h("span", { class: "mono muted" }, s.path)),
        s.integrated ? badge("ok", "Loads GitPersona") : badge("warn", "Not set up")))),
    missing.length > 0
      ? [h("p", { class: "muted" }, "GitPersona adds a marked block to each shell's startup file; uninstall removes it exactly."), form]
      : null);
}
