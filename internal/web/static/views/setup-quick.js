// The first page of setup: everything GitPersona found, organized into a
// configuration that is applied with one click. The three steps of setup.js
// remain for adjusting it.

import { api } from "../api.js";
import { h, replace, badge, busy } from "../dom.js";
import { diffBlock } from "./apply.js";

// QUICK is the wizard step of this page; the detailed steps are 0, 1 and 2.
export const QUICK = -1;

const NAME_NOTES = {
  login: "The account shows no public name, so the name is its login.",
  global: "Your includeIf setup sets no name, so the name is your global Git name.",
};

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
  const preview = filesPreview(ctx);
  // Every choice changes the files, so an open preview follows it.
  const set = (next) => {
    ctx.set(next);
    preview.refresh();
  };
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, "Your setup is ready"),
    h("p", { class: "muted" }, foundText(wizard)),
    scanLimitNotice(wizard),
    wizard.scanTooDeep ? h("p", { class: "muted" }, "Repositories nested more than five folders deep were not looked at; " +
      "add their directories under Adjust step by step.") : null,
    included.length === 0
      ? h("div", { class: "notice warn" }, "No profile is chosen. Use Adjust step by step to choose at least one.")
      : h("div", { class: "grid" }, included.map((p) => profileSummary(p, p.account === wizard.defaultAccount))),
    outsideText(wizard),
    leftOutChoices(ctx.get, ctx.set, () => ctx.update({})),
    wizard.unassigned.length > 0 ? ctx.unassigned() : null,
    pinChoices(ctx.get, set),
    oldRulesChoice(ctx.get, set),
    shellChoices(ctx.get, set),
    preview.element,
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
    NAME_NOTES[p.name_source] ? h("p", { class: "muted" }, NAME_NOTES[p.name_source]) : null,
    p.ssh_key ? h("p", { class: "muted" }, "SSH key ", h("span", { class: "mono" }, p.ssh_key)) : null,
    p.signing?.method ? h("p", { class: "muted" }, SIGNING[p.signing.method] ?? p.signing.method) : null,
    dirs.length === 0 ? h("p", { class: "empty" }, "No directories yet; add them under Adjust or later with gitpersona use.")
      : h("ul", { class: "dirs" }, dirs.map((d) => h("li", {}, h("span", { class: "mono" }, d.dir), h("span", { class: "muted" }, ` · ${d.why}`)))),
    profileNotes(p));
}

// profileNotes says what of the existing setup was left out of a profile.
export function profileNotes(p) {
  return (p.notes ?? []).map((note) => h("p", { class: "muted" }, badge("warn"), ` ${note}.`));
}

// leftOutChoices lists the identities of the existing setup that no account
// could be tied to, and lets the user choose the account for each.
// redraw shows the page again after a choice.
export function leftOutChoices(get, set, redraw) {
  const wizard = get();
  if (wizard.unmatched.length === 0) return null;
  // A profile that already has an identity of the setup keeps it.
  const chosen = wizard.profiles.map((p, i) => [p, i]).filter(([p]) => p.include && p.source !== "existing");
  return h("div", { class: "notice warn" },
    h("p", {}, badge("warn"), " GitPersona could not tie these identities of your Git configuration to an account signed in to gh. " +
      "Choose an account to make one a profile:"),
    h("ul", {}, wizard.unmatched.map((u, k) => h("li", {},
      `${u.name || "(no name)"} <${u.email}> · ${u.dirs.join(", ")} (${u.source})`,
      h("p", {}, "Your rule keeps applying there.",
        u.account_from ? ` What it does not set, such as the account for pushes, comes from profile ${u.account_from}.` : ""),
      chosen.length === 0 ? h("p", {}, "Every chosen profile already uses an identity of your setup.")
        : accountSelect(u, chosen, (i) => {
          set(adoptIdentity(get(), k, i));
          redraw();
        })))));
}

// accountSelect chooses the profile for a left-out identity.
function accountSelect(u, chosen, onChoose) {
  const select = h("select", { "aria-label": `Account for ${u.email}` },
    h("option", { value: "" }, "Leave it out"),
    chosen.map(([p, i]) => h("option", { value: String(i) }, `Use it for ${p.account}`)));
  select.addEventListener("change", () => {
    if (select.value !== "") onChoose(Number(select.value));
  });
  return select;
}

// adoptIdentity gives profile i the left-out identity k: its name, email,
// directories and keys, and offers to remove the rules it then takes over.
function adoptIdentity(w, k, i) {
  const u = w.unmatched[k];
  const profiles = w.profiles.map((p, j) => (j !== i ? p : {
    ...p,
    // An identity without a name committed with the global Git name.
    name: u.name || w.global?.name || p.name,
    name_source: u.name ? "existing" : w.global?.name ? "global" : p.name_source,
    email: u.email,
    email_source: "existing",
    source: "existing",
    ssh_key: u.ssh_key || p.ssh_key,
    signing: u.signing?.method ? u.signing : p.signing,
    dirs: [...p.dirs, ...u.dirs.filter((d) => !p.dirs.some((x) => x.dir === d))
      .map((d) => ({ dir: d, on: true, why: "your existing setup" }))],
  }));
  const dirs = [...w.oldRules.dirs, ...u.replaceable.filter((d) => !w.oldRules.dirs.includes(d))];
  return { ...w, profiles, unmatched: w.unmatched.filter((_, j) => j !== k), oldRules: { ...w.oldRules, dirs } };
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
  return h("p", { class: "muted" }, `Outside these directories, commits use ${chosen.id} (${chosen.name} <${chosen.email}>)`,
    today ? ` instead of ${today} (choose "No default" under Adjust to keep it).` : ".");
}

// scanLimitNotice says which folders the scan did not finish, if any.
export function scanLimitNotice(wizard) {
  if (!wizard.scanIncomplete) return null;
  const where = wizard.scanUnfinished?.length ? ` before it finished ${wizard.scanUnfinished.join(", ")}` : "";
  return h("div", { class: "notice warn" }, badge("warn"),
    ` Looking for repositories stopped at its time limit${where}, so repositories it did not reach are not included. ` +
    "Add their folders to a profile.");
}

// pinNeeded reports whether a pin changes anything: one for a repository
// outside every directory only matters while its account is not the default.
export function pinNeeded(w, pin) {
  return !pin.outside || pin.account !== w.defaultAccount;
}

// pinChoices lists repositories whose own settings name another profile
// than their folder; each can be left out.
export function pinChoices(get, set) {
  const wizard = get();
  if (!wizard.pins.some((pin) => pinNeeded(wizard, pin))) return null;
  return h("fieldset", {},
    h("legend", {}, "Repositories that differ from their folder"),
    wizard.pins.map((pin, k) => {
      if (!pinNeeded(wizard, pin)) return null; // the default already gives it this profile
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

// filesPreview loads the diffs only when asked, so the page opens at once;
// refresh loads them again while they are shown.
function filesPreview(ctx) {
  const body = h("div", {}, h("p", { class: "muted" }, "Loading…"));
  const element = h("details", { class: "change" }, h("summary", {}, "Show the files that change"), body);
  let latest = 0; // only the newest answer is shown
  const load = () => {
    const asked = ++latest;
    body.setAttribute("aria-busy", "true");
    api.setupPreview(ctx.planOf(ctx.get())).then(({ data }) => {
      if (asked !== latest) return;
      replace(body, data.map((c) => h("details", { class: "change" },
        h("summary", {}, badge(c.action === "create" ? "info" : "warn", c.action === "create" ? "New file" : "Changed"),
          h("span", { class: "mono" }, c.path)),
        diffBlock(c.diff))));
    }).catch((err) => {
      if (asked === latest) replace(body, h("div", { class: "notice fail", role: "alert" }, err.message));
    }).finally(() => {
      if (asked === latest) body.removeAttribute("aria-busy");
    });
  };
  element.addEventListener("toggle", () => {
    if (element.open) load();
  });
  return { element, refresh: () => { if (element.open) load(); } };
}
