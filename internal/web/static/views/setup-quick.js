// The first page of setup: everything GitPersona found, organized into a
// configuration that is applied with one click. The three steps of setup.js
// remain for adjusting it.

import { t } from "../i18n.js";
import { api } from "../api.js";
import { h, replace, badge, busy } from "../dom.js";
import { diffBlock } from "./apply.js";

// QUICK is the wizard step of this page; the detailed steps are 0, 1 and 2.
export const QUICK = -1;

const NAME_NOTES = { login: "quick.nameLogin", global: "quick.nameGlobal" };

const SIGNING = { ssh: "quick.signsSSH", gpg: "quick.signsGPG", none: "quick.neverSigns" };

// quickView shows the proposal. ctx gives the wizard state and its actions:
// get() returns the current state (handlers must read it when they run, not
// when the page was drawn), set(next), planOf(state), update(changes),
// apply() and unassigned().
export function quickView(ctx) {
  const wizard = ctx.get();
  const included = wizard.profiles.filter((p) => p.include);
  const apply = h("button", { type: "submit", class: "primary", disabled: included.length === 0 }, t("apply.apply"));
  const adjust = h("button", { type: "button", onclick: () => ctx.update({ step: 0 }) }, t("quick.adjust"));
  const preview = filesPreview(ctx);
  // Every choice changes the files, so an open preview follows it.
  const set = (next) => {
    ctx.set(next);
    preview.refresh();
  };
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, t("quick.ready")),
    h("p", { class: "muted" }, foundText(wizard)),
    scanLimitNotice(wizard),
    githubNotice(wizard),
    wizard.scanTooDeep ? h("p", { class: "muted" }, t("quick.tooDeep")) : null,
    included.length === 0
      ? h("div", { class: "notice warn" }, t("quick.noneChosen"))
      : h("div", { class: "grid" }, included.map((p) => profileSummary(p, p.key === wizard.defaultKey))),
    outsideText(wizard),
    leftOutChoices(ctx.get, ctx.set, () => ctx.update({})),
    wizard.shadowed?.length ? h("div", { class: "notice warn" }, badge("warn"), " ", t("quick.foldersChange"),
      h("ul", {}, wizard.shadowed.map((text) => h("li", {}, text)))) : null,
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
  return wizard.roots ? t("quick.foundWithRoots", { roots: wizard.roots }) : t("quick.found");
}

function profileSummary(p, isDefault) {
  const dirs = p.dirs.filter((d) => d.on);
  return h("article", { class: "card", "aria-label": t("quick.profileLabel", { id: p.id }) },
    h("div", { class: "card-head" }, h("h4", {}, p.id), isDefault ? badge("info", t("account.default")) : null),
    h("p", {}, `${p.name} <${p.email}>`),
    onGitHub(p) ? null : h("p", { class: "muted" }, t("quick.account", { account: accountLabel(p) })),
    NAME_NOTES[p.name_source] ? h("p", { class: "muted" }, t(NAME_NOTES[p.name_source])) : null,
    p.ssh_key ? h("p", { class: "muted" }, t("fact.sshKey"), " ", h("span", { class: "mono" }, p.ssh_key)) : null,
    p.signing?.method ? h("p", { class: "muted" }, SIGNING[p.signing.method] ? t(SIGNING[p.signing.method]) : p.signing.method) : null,
    dirs.length === 0 ? h("p", { class: "empty" }, t("quick.noDirs"))
      : h("ul", { class: "dirs" }, dirs.map((d) => h("li", {}, h("span", { class: "mono" }, d.dir), h("span", { class: "muted" }, ` · ${d.why}`)))),
    profileNotes(p));
}

// profileNotes says what of the existing setup was left out of a profile.
export function profileNotes(p) {
  return (p.notes ?? []).map((note) => h("p", { class: "muted" }, badge("warn"), ` ${note}`));
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
    h("p", {}, badge("warn"), " ", t("quick.unmatched")),
    h("ul", {}, wizard.unmatched.map((u, k) => h("li", {},
      `${u.name || t("quick.noName")} <${u.email}> · ${u.dirs.join(", ")} (${u.source})`,
      u.overridden_by || wizard.defaultKey
        ? h("p", {}, u.overridden_by ? t("quick.overriddenBy", { id: u.overridden_by }) : t("quick.overriddenByDefault"))
        : h("p", {}, t("quick.ruleKeeps"),
          u.replaced_in?.length ? t("quick.exceptIn", { dirs: u.replaced_in.join(", ") }) : ""),
      chosen.length === 0 ? h("p", {}, t("quick.allUsed"))
        : accountSelect(u, chosen, (i) => {
          set(adoptIdentity(get(), k, i));
          redraw();
        })))));
}

// accountSelect chooses the profile for a left-out identity.
function accountSelect(u, chosen, onChoose) {
  const select = h("select", { "aria-label": t("quick.accountFor", { email: u.email }) },
    h("option", { value: "" }, t("quick.leaveOut")),
    chosen.map(([p, i]) => h("option", { value: String(i) }, t("quick.useFor", { account: accountLabel(p) }))));
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
      .map((d) => ({ dir: d, on: true, why: t("why.existing") }))],
  }));
  const dirs = [...w.oldRules.dirs, ...u.replaceable.filter((d) => !w.oldRules.dirs.includes(d))];
  return { ...w, profiles, unmatched: w.unmatched.filter((_, j) => j !== k), oldRules: { ...w.oldRules, dirs } };
}

// outsideText says which identity applies outside every directory.
function outsideText(wizard) {
  const chosen = wizard.profiles.find((p) => p.include && p.key === wizard.defaultKey);
  const g = wizard.global ?? {};
  const today = g.email ? `${g.name || t("quick.noName")} <${g.email}>` : "";
  if (!chosen) {
    return h("p", { class: "muted" }, today ? t("quick.outsideKeep", { who: today }) : t("quick.outsideGit"),
      wizard.ownRules?.length ? t("quick.outsideOwnRules") : t("quick.period"));
  }
  return h("p", { class: "muted" }, t("quick.outsideUse", { id: chosen.id, who: `${chosen.name} <${chosen.email}>` }),
    today ? t("quick.insteadOf", { who: today }) : t("quick.period"));
}

// githubNotice says why no GitHub profile is proposed, when gh could not
// be asked but other hosts' profiles are.
function githubNotice(wizard) {
  if (!wizard.githubUnavailable) return null;
  return h("div", { class: "notice warn" }, badge("warn"), " ", t("quick.githubUnavailable", { why: wizard.githubUnavailable }));
}

// scanLimitNotice says which folders the scan did not finish, if any.
export function scanLimitNotice(wizard) {
  if (!wizard.scanIncomplete) return null;
  return h("div", { class: "notice warn" }, badge("warn"), " ",
    wizard.scanUnfinished?.length ? t("quick.scanStoppedIn", { dirs: wizard.scanUnfinished.join(", ") }) : t("quick.scanStopped"));
}

// pinNeeded reports whether a pin changes anything: one for a repository
// outside every directory only matters while its account is not the default.
export function pinNeeded(w, pin) {
  return !pin.outside || pin.key !== w.defaultKey;
}

// onGitHub reports whether a profile is for GitHub, where repositories on
// disk are matched to accounts.
export function onGitHub(p) {
  return !p.host || p.host === "github.com";
}

// accountLabel names a profile's account, with its host when that is not
// GitHub.
export function accountLabel(p) {
  return onGitHub(p) ? p.account : t("account.onHost", { account: p.account, host: p.host });
}

// pinChoices lists repositories whose own settings name another profile
// than their folder; each can be left out.
export function pinChoices(get, set) {
  const wizard = get();
  if (!wizard.pins.some((pin) => pinNeeded(wizard, pin))) return null;
  return h("fieldset", {},
    h("legend", {}, t("quick.pinsLegend")),
    wizard.pins.map((pin, k) => {
      if (!pinNeeded(wizard, pin)) return null; // the default already gives it this profile
      const box = h("input", { type: "checkbox", checked: pin.on });
      box.addEventListener("change", () => toggle(get, set, "pins", k, box.checked));
      const id = wizard.profiles.find((p) => p.key === pin.key)?.id ?? t("quick.leftOutProfile");
      return h("label", { class: "check" }, box, h("span", {}, t("quick.pinBefore"), h("span", { class: "mono" }, pin.repo), t("quick.pinTo", { id })),
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
    h("legend", {}, t("quick.oldSetup")),
    oldRules.dirs.length > 0 ? h("label", { class: "check" }, box, t("quick.removeOldRules", { dirs: oldRules.dirs.join(", ") })) : null,
    oldRules.kept.map((k) => k.overridden_by
      ? h("p", {}, badge("warn"), " ", t("quick.keptLost", { dir: k.dir, why: k.why, id: k.overridden_by }))
      : h("p", { class: "muted" }, t("quick.keptStays", { dir: k.dir, why: k.why }),
        k.replaced_in?.length ? t("quick.exceptIn", { dirs: k.replaced_in.join(", ") }) : "")));
}

// shellChoices offers the shell integration, chosen by default.
export function shellChoices(get, set) {
  const wizard = get();
  if (wizard.shells.length === 0) return null;
  return h("fieldset", {},
    h("legend", {}, t("quick.shell")),
    wizard.shells.map((sh, k) => {
      const box = h("input", { type: "checkbox", checked: sh.on });
      box.addEventListener("change", () => toggle(get, set, "shells", k, box.checked));
      return h("label", { class: "check" }, box, t("quick.loadShell", { shell: sh.kind }));
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
  const body = h("div", {}, h("p", { class: "muted" }, t("quick.loading")));
  const element = h("details", { class: "change" }, h("summary", {}, t("quick.showFiles")), body);
  let latest = 0; // only the newest answer is shown
  const load = () => {
    const asked = ++latest;
    body.setAttribute("aria-busy", "true");
    api.setupPreview(ctx.planOf(ctx.get())).then(({ data }) => {
      if (asked !== latest) return;
      replace(body, data.map((c) => h("details", { class: "change" },
        h("summary", {}, badge(c.action === "create" ? "info" : "warn", c.action === "create" ? t("file.new") : t("file.changed")),
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
