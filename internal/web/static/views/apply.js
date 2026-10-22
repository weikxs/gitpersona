// Review & apply: shows every file apply would change, as a diff, before
// anything is written. Removing files needs an extra confirmation.

import { api } from "../api.js";
import { h, replace, badge, busy, confirm } from "../dom.js";

const ACTION_BADGES = { create: ["info", "New file"], update: ["warn", "Changed"], remove: ["fail", "Removed"] };

export function renderApply(section, { run }) {
  const body = h("div", { "aria-busy": "true" }, h("p", {}, "Comparing with the files on disk…"));
  section.append(
    h("p", { class: "muted" }, "GitPersona writes Git's configuration from your profiles. " +
      "Review the changes below; your files are backed up before anything is written."),
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
    return h("div", { class: "notice ok" }, badge("ok", "Up to date"), " Git's configuration already matches your profiles.");
  }
  const removals = changes.filter((c) => c.action === "remove");
  const button = h("button", { type: "button", class: "primary" }, `Apply ${changes.length} change${changes.length === 1 ? "" : "s"}`);
  button.addEventListener("click", () => busy(button, async () => {
    if (removals.length > 0 && !(await confirm("Remove files?",
      `Applying removes ${removals.map((c) => c.path).join(", ")}. They are backed up first.`, "Apply"))) {
      return;
    }
    await run(() => api.apply(true), "Applied. Git now uses your profiles.");
  }));
  return h("div", {},
    changes.map((c, i) => change(c, i === 0)),
    h("div", { class: "actions" }, button));
}

function change(c, open) {
  const [kind, label] = ACTION_BADGES[c.action] ?? ["info", c.action];
  return h("details", { class: "change", open },
    h("summary", {}, badge(kind, label), h("span", { class: "mono" }, c.path)),
    diffBlock(c.diff));
}

// diffBlock shows a unified diff; each line keeps its +, - or space prefix,
// so additions and removals are clear without color.
export function diffBlock(diff) {
  const lines = diff.replace(/\n$/, "").split("\n");
  return h("pre", { class: "diff", tabindex: "0", "aria-label": "Changes" },
    lines.map((line) => h("span", { class: lineClass(line) }, line || " ")));
}

function lineClass(line) {
  if (line.startsWith("+++") || line.startsWith("---")) return "file";
  if (line.startsWith("@@")) return "hunk";
  if (line.startsWith("+")) return "add";
  if (line.startsWith("-")) return "del";
  return "";
}
