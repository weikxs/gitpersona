// Shells: which shell is the login shell and which one runs now, and whether
// each installed shell loads GitPersona (for the gh account and prompt).

import { api } from "../api.js";
import { h, replace, badge, busy, confirm } from "../dom.js";
import { diffBlock } from "./apply.js";
import { factList } from "./overview.js";

// sourceText says in plain words where a shell was found.
function sourceText(source = "") {
  if (source === "dscl" || source === "getent") return "from your account settings";
  if (source === "$SHELL") return "from the SHELL variable";
  if (source.startsWith("process")) return "the shell that started GitPersona";
  return source;
}

function describe(shell) {
  if (!shell?.kind) return "Not detected";
  return [shell.kind, shell.path, shell.source ? `(${sourceText(shell.source)})` : ""].filter(Boolean).join(" ");
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
  const changes = h("div", { class: "stack" });
  const form = h("form", { class: "stack" },
    h("fieldset", {},
      h("legend", {}, "Load GitPersona in"),
      missing.map((s, i) => h("label", { class: "check" }, boxes[i], `${s.kind} (${s.path})`))),
    h("div", { class: "actions" }, submit),
    changes);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const chosen = boxes.filter((b) => b.checked).map((b) => b.value);
    if (chosen.length === 0) return;
    busy(submit, async () => {
      // Show which startup files change before anything is written.
      let files;
      try {
        files = (await api.previewShells(chosen)).data;
      } catch (err) {
        replace(changes, h("div", { class: "notice fail", role: "alert" }, err.message));
        return;
      }
      replace(changes, files.map((c) => h("details", { class: "change" },
        h("summary", {}, h("span", { class: "mono" }, c.path)), diffBlock(c.diff))));
      const names = files.map((c) => `${c.path} (${c.action === "create" ? "new" : "changed"})`).join("\n");
      if (files.length > 0 && !(await confirm("Change these startup files?",
        `${names}\n\nGitPersona adds one marked block to each; the changes are listed below the button. ` +
        "Existing files are backed up first.", "Add"))) {
        return;
      }
      await run(() => api.installShells(chosen), `Added GitPersona to ${chosen.join(", ")}. Open a new terminal to use it.`);
    });
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
