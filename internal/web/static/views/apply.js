// Review & apply: shows every file apply would change, as a diff, before
// anything is written. Removing files needs an extra confirmation.

import { t, tn } from "../i18n.js";
import { api } from "../api.js";
import { h, replace, badge, busy, confirm } from "../dom.js";

const ACTION_BADGES = { create: ["info", "file.new"], update: ["warn", "file.changed"], remove: ["fail", "file.removed"] };

export function renderApply(section, { run }) {
  const body = h("div", { "aria-busy": "true" }, h("p", {}, t("apply.comparing")));
  section.append(
    h("p", { class: "lead" }, t("apply.lead")),
    body);
  load(body, run);
}

async function load(body, run) {
  try {
    const { data } = await api.preview();
    replace(body, preview(data, run));
  } catch (err) {
    replace(body, h("div", { class: "notice fail", role: "alert" }, err.message));
  }
  body.removeAttribute("aria-busy");
}

function preview(changes, run) {
  if (changes.length === 0) {
    return h("div", { class: "notice ok" }, badge("ok", t("apply.upToDate")), " ", t("apply.matches"));
  }
  const removals = changes.filter((c) => c.action === "remove");
  const button = h("button", { type: "button", class: "primary" }, tn(changes.length, "apply.buttonOne", "apply.buttonMany"));
  button.addEventListener("click", () => busy(button, async () => {
    if (removals.length > 0 && !(await confirm(t("apply.removeAsk"),
      t("apply.removeText", { files: removals.map((c) => c.path).join(", ") }), t("apply.apply")))) {
      return;
    }
    await run(() => api.apply(true), t("apply.done"));
  }));
  return h("div", {},
    changes.map((c, i) => change(c, i === 0)),
    h("div", { class: "actions" }, button));
}

function change(c, open) {
  const [kind, label] = ACTION_BADGES[c.action] ?? ["info", c.action];
  return h("details", { class: "change", open },
    h("summary", {}, badge(kind, t(label)), h("span", { class: "mono" }, c.path)),
    diffBlock(c.diff));
}

// diffBlock shows a unified diff; each line keeps its +, - or space prefix,
// so additions and removals are clear without color.
export function diffBlock(diff) {
  const lines = diff.replace(/\n$/, "").split("\n");
  return h("pre", { class: "diff", tabindex: "0", "aria-label": t("file.changes") },
    lines.map((line) => h("span", { class: lineClass(line) }, line || " ")));
}

function lineClass(line) {
  if (line.startsWith("+++") || line.startsWith("---")) return "file";
  if (line.startsWith("@@")) return "hunk";
  if (line.startsWith("+")) return "add";
  if (line.startsWith("-")) return "del";
  return "";
}
