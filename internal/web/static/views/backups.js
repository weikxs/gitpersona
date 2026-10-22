// Backups: every change made by GitPersona is backed up first; restoring
// one puts those files back and regenerates Git's configuration.

import { api } from "../api.js";
import { h, replace, busy, confirm } from "../dom.js";

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" });

export function renderBackups(section, { run }) {
  const body = h("div", { "aria-busy": "true" }, h("p", {}, "Loading backups…"));
  section.append(
    h("p", { class: "muted" }, "The newest backups are kept, along with the very first one, which holds your files from before GitPersona."),
    body);
  load(body, run);
}

async function load(body, run) {
  try {
    const { data } = await api.backups();
    replace(body, list(data, run));
  } catch (err) {
    replace(body, h("div", { class: "notice fail", role: "alert" }, err.message));
  }
  body.removeAttribute("aria-busy");
}

function list(backups, run) {
  if (backups.length === 0) {
    return h("p", { class: "empty" }, "No backups yet. One is made before each change.");
  }
  const newestFirst = [...backups].sort((a, b) => b.created.localeCompare(a.created));
  return h("ul", { class: "rows" }, newestFirst.map((b) => {
    const when = dateFormat.format(new Date(b.created));
    const button = h("button", { type: "button", class: "small", "aria-label": `Restore the backup from ${when}` }, "Restore");
    button.addEventListener("click", () => busy(button, async () => {
      const ok = await confirm("Restore this backup?",
        `This puts back ${b.files.length} file${b.files.length === 1 ? "" : "s"} as they were ${b.command ? `before ${b.command}` : `on ${when}`}. ` +
        "The current files are backed up first, so this can be undone too.", "Restore");
      if (ok) await run(() => api.rollback(b.name), `Restored the backup from ${when}.`);
    }));
    return h("li", {},
      h("div", {},
        h("strong", {}, when), " ",
        h("span", { class: "muted" }, b.command ? `before ${b.command}` : ""),
        h("div", { class: "mono muted" }, b.files.join(", "))),
      button);
  }));
}
