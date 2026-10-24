// Shells: which shell is the login shell and which one runs now, and whether
// each installed shell loads GitPersona (for the gh account and prompt).

import { t } from "../i18n.js";
import { api } from "../api.js";
import { h, replace, badge, busy, confirm } from "../dom.js";
import { diffBlock } from "./apply.js";
import { factList } from "./overview.js";

// sourceText says in plain words where a shell was found.
function sourceText(source = "") {
  if (source === "dscl" || source === "getent") return t("shells.fromAccount");
  if (source === "$SHELL") return t("shells.fromVariable");
  if (source.startsWith("process")) return t("shells.fromProcess");
  return source;
}

function describe(shell) {
  if (!shell?.kind) return t("shells.notDetected");
  return [shell.kind, shell.path, shell.source ? `(${sourceText(shell.source)})` : ""].filter(Boolean).join(" ");
}

export function renderShells(section, { state, run }) {
  const shells = state.shells;
  section.append(
    h("div", { class: "card" },
      h("h3", {}, t("shells.detected")),
      factList([[t("shells.login"), describe(shells.default)], [t("shells.current"), describe(shells.current)]])),
    installedCard(shells.installed, run));
}

function installedCard(installed, run) {
  if (installed.length === 0) {
    return h("div", { class: "card" }, h("h3", {}, t("shells.installed")),
      h("p", { class: "empty" }, t("shells.none")));
  }
  const missing = installed.filter((s) => !s.integrated);
  const boxes = missing.map((s) => h("input", { type: "checkbox", name: "shell", value: s.kind, checked: true }));
  const submit = h("button", { type: "submit", class: "primary" }, t("shells.add"));
  const changes = h("div", { class: "stack" });
  const form = h("form", { class: "stack" },
    h("fieldset", {},
      h("legend", {}, t("shells.loadIn")),
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
      const names = files.map((c) => `${c.path} (${c.action === "create" ? t("shells.new") : t("shells.changed")})`).join("\n");
      if (files.length > 0 && !(await confirm(t("shells.changeAsk"),
        `${names}\n\n${t("shells.changeText")}`, t("shells.addButton")))) {
        return;
      }
      await run(() => api.installShells(chosen), t("shells.added", { shells: chosen.join(", ") }));
    });
  });
  return h("div", { class: "card" },
    h("h3", {}, t("shells.installed")),
    h("ul", { class: "rows" }, installed.map((s) =>
      h("li", {}, h("span", {}, s.kind, " ", h("span", { class: "mono muted" }, s.path)),
        s.integrated ? badge("ok", t("shells.loads")) : badge("warn", t("shells.notSetUp"))))),
    missing.length > 0
      ? [h("p", { class: "muted" }, t("shells.blockNote")), form]
      : null);
}
