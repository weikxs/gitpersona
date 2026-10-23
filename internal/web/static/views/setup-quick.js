// The first page of setup: everything GitPersona found, organized into a
// configuration that is applied with one click. The three steps of setup.js
// remain for adjusting it.

import { api } from "../api.js";
import { h, replace, badge, busy } from "../dom.js";
import { diffBlock } from "./apply.js";

// QUICK is the wizard step of this page; the detailed steps are 0, 1 and 2.
export const QUICK = -1;

const SIGNING = { ssh: "signs commits with SSH", gpg: "signs commits with GPG", none: "never signs" };

// quickView shows the proposal. ctx gives the wizard state and its actions:
// get() returns the current state (handlers must read it when they run, not
// when the page was drawn), set(next), planOf(state), update(changes),
// apply() and unassigned().
export function quickView(ctx) {
  const wizard = ctx.get();
  const included = wizard.profiles.filter((p) => p.include);
  const apply = h("button", { type: "submit", class: "primary", disabled: included.length === 0 }, "Apply");
  const adjust = h("button", { type: "button", onclick: () => ctx.update({ step: 0 }) }, "Adjust step by step");
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, "Your setup is ready"),
    h("p", { class: "muted" }, foundText(wizard)),
    included.length === 0
      ? h("div", { class: "notice warn" }, "No profile is chosen. Use Adjust step by step to choose at least one.")
      : h("div", { class: "grid" }, included.map((p) => profileSummary(p, p.account === wizard.defaultAccount))),
    outsideText(wizard),
    wizard.unassigned.length > 0 ? ctx.unassigned() : null,
    pinChoices(ctx.get, ctx.set),
    oldRulesChoice(ctx.get, ctx.set),
    shellChoices(ctx.get, ctx.set),
    filesPreview(ctx),
    h("div", { class: "actions" }, adjust, apply));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    busy(apply, ctx.apply);
  });
  return form;
}

function foundText(wizard) {
  const where = wizard.roots ? `, and the repositories in ${wizard.roots}` : "";
  return `GitPersona looked at the accounts signed in to gh, your Git settings, ~/.ssh/config${where}, ` +
    "and put them together below. Nothing is written until you apply; your files are backed up first.";
}

function profileSummary(p, isDefault) {
  const dirs = p.dirs.filter((d) => d.on);
  return h("article", { class: "card", "aria-label": `Profile ${p.id}` },
    h("div", { class: "card-head" }, h("h4", {}, p.id), isDefault ? badge("info", "Default") : null),
    h("p", {}, `${p.name} <${p.email}>`),
    p.ssh_key ? h("p", { class: "muted" }, "SSH key ", h("span", { class: "mono" }, p.ssh_key)) : null,
    p.signing?.method ? h("p", { class: "muted" }, SIGNING[p.signing.method] ?? p.signing.method) : null,
    dirs.length === 0 ? h("p", { class: "empty" }, "No directories yet; add them under Adjust or later with gitpersona use.")
      : h("ul", { class: "dirs" }, dirs.map((d) => h("li", {}, h("span", { class: "mono" }, d.dir), h("span", { class: "muted" }, ` · ${d.why}`)))));
}

// outsideText says which identity applies outside every directory.
function outsideText(wizard) {
  const chosen = wizard.profiles.find((p) => p.include && p.account === wizard.defaultAccount);
  const g = wizard.global ?? {};
  const today = g.email ? `${g.name || "(no name)"} <${g.email}>` : "";
  if (!chosen) {
    return h("p", { class: "muted" }, today ? `Outside these directories, commits keep using ${today}.`
      : "Outside these directories, Git's own settings apply.");
  }
  return h("p", { class: "muted" }, `Outside these directories, commits use ${chosen.id}`,
    today ? ` instead of ${today} (choose "No default" under Adjust to keep it).` : ".");
}

// pinChoices lists repositories whose own settings name another profile
// than their folder; each can be left out.
export function pinChoices(get, set) {
  const wizard = get();
  if (wizard.pins.length === 0) return null;
  return h("fieldset", {},
    h("legend", {}, "Repositories that differ from their folder"),
    wizard.pins.map((pin, k) => {
      const box = h("input", { type: "checkbox", checked: pin.on });
      box.addEventListener("change", () => toggle(get, set, "pins", k, box.checked));
      const id = wizard.profiles.find((p) => p.account === pin.account)?.id ?? pin.account;
      return h("label", { class: "check" }, box, h("span", {}, "Pin ", h("span", { class: "mono" }, pin.repo), ` to ${id}`),
        h("span", { class: "muted" }, ` · ${pin.why}`));
    }));
}

// oldRulesChoice offers to remove the hand-made includeIf rules the
// profiles take over, chosen by default.
export function oldRulesChoice(get, set) {
  const { oldRules } = get();
  if (oldRules.dirs.length === 0 && oldRules.kept.length === 0) return null;
  const box = h("input", { type: "checkbox", checked: oldRules.on });
  box.addEventListener("change", () => {
    const w = get();
    set({ ...w, oldRules: { ...w.oldRules, on: box.checked } });
  });
  return h("fieldset", {},
    h("legend", {}, "Your old setup"),
    oldRules.dirs.length > 0 ? h("label", { class: "check" }, box,
      `Remove the includeIf rules for ${oldRules.dirs.join(", ")} from your Git configuration; the profiles take them over`) : null,
    oldRules.kept.map((k) => h("p", { class: "muted" }, `The includeIf rule for ${k.dir} stays: ${k.why}.`)));
}

// shellChoices offers the shell integration, chosen by default.
export function shellChoices(get, set) {
  const wizard = get();
  if (wizard.shells.length === 0) return null;
  return h("fieldset", {},
    h("legend", {}, "Shell"),
    wizard.shells.map((sh, k) => {
      const box = h("input", { type: "checkbox", checked: sh.on });
      box.addEventListener("change", () => toggle(get, set, "shells", k, box.checked));
      return h("label", { class: "check" }, box, `Load GitPersona in ${sh.kind}, so gh and your prompt follow the directory`);
    }));
}

// toggle turns item k of a list in the current state on or off.
function toggle(get, set, list, k, on) {
  const w = get();
  set({ ...w, [list]: w[list].map((x, j) => (j === k ? { ...x, on } : x)) });
}

// filesPreview loads the diffs only when asked, so the page opens at once.
function filesPreview(ctx) {
  const body = h("div", {}, h("p", { class: "muted" }, "Loading…"));
  const details = h("details", { class: "change" }, h("summary", {}, "Show the files that change"), body);
  details.addEventListener("toggle", () => {
    if (!details.open) return;
    api.setupPreview(ctx.planOf(ctx.get())).then(({ data }) => {
      replace(body, data.map((c) => h("details", { class: "change" },
        h("summary", {}, badge(c.action === "create" ? "info" : "warn", c.action === "create" ? "New file" : "Changed"),
          h("span", { class: "mono" }, c.path)),
        diffBlock(c.diff))));
    }).catch((err) => replace(body, h("div", { class: "notice fail", role: "alert" }, err.message)));
  });
  return details;
}
