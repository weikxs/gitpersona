// Backups: every change made by GitPersona is backed up first; restoring
// one puts those files back and regenerates Git's configuration.

import { t, tn, dateTime } from "../i18n.js";
import { api } from "../api.js";
import { h, replace, busy, confirm } from "../dom.js";

// COMMANDS describes what made a backup in words, not command names; the
// backup was taken before that change.
const COMMANDS = [
  [/^use (.+)$/, (id) => t("backup.use", { id })],
  [/^remove profile (.+)$/, (id) => t("backup.removeProfile", { id })],
  [/^remove old includeIf rules$/, () => t("backup.removeOldRules")],
  [/^remove (.+)$/, (dir) => t("backup.removeDir", { dir })],
  [/^pin (.+)$/, (id) => t("backup.pin", { id })],
  [/^unpin$/, () => t("backup.unpin")],
  [/^add profile (.+)$/, (id) => t("backup.addProfile", { id })],
  [/^update profile (.+)$/, (id) => t("backup.updateProfile", { id })],
  [/^set default (.*)$/, (id) => (id ? t("backup.setDefault", { id }) : t("backup.removeDefault"))],
  [/^setup/, () => t("backup.setup")],
  [/^install-shell$/, () => t("backup.installShell")],
  [/^uninstall$/, () => t("backup.uninstall")],
  [/^apply$/, () => t("backup.apply")],
  [/^rollback/, () => t("backup.rollback")],
];

function describe(command) {
  for (const [pattern, words] of COMMANDS) {
    const match = pattern.exec(command);
    if (match) return words(...match.slice(1));
  }
  return command;
}

export function renderBackups(section, { run }) {
  const body = h("div", { "aria-busy": "true" }, h("p", {}, t("backup.loading")));
  section.append(
    h("p", { class: "lead" }, t("backup.lead")),
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
    return h("p", { class: "empty" }, t("backup.none"));
  }
  const newestFirst = [...backups].sort((a, b) => b.created.localeCompare(a.created));
  return h("ul", { class: "rows" }, newestFirst.map((b) => {
    const when = dateTime(new Date(b.created));
    const before = b.command ? t("backup.before", { what: describe(b.command) }) : t("backup.on", { when });
    const button = h("button", { type: "button", class: "small", "aria-label": t("backup.restoreLabel", { when }) }, t("backup.restore"));
    button.addEventListener("click", () => busy(button, async () => {
      const ok = await confirm(t("backup.restoreAsk"),
        `${tn(b.files.length, "backup.putBackOne", "backup.putBackMany", { when: before })} ${t("backup.undoable")}`, t("backup.restore"));
      if (ok) await run(() => api.rollback(b.name), t("backup.restored", { when }));
    }));
    return h("li", {},
      h("div", {},
        h("strong", {}, when), " ",
        h("span", { class: "muted" }, b.command ? before : ""),
        h("div", { class: "mono muted" }, b.files.join(", "))),
      button);
  }));
}
