// First-run setup in three steps: confirm identities, confirm directories,
// then review every file, apply and check. Nothing is written before the
// last step.

import { t, tn } from "../i18n.js";
import { api, normalizePath } from "../api.js";
import { h, replace, badge, field, busy, status, clearStatus } from "../dom.js";
import { doctorCard, configProblem } from "./overview.js";
import { diffBlock } from "./apply.js";
import { quickView, pinChoices, shellChoices, oldRulesChoice, leftOutChoices, profileNotes, pinNeeded, onGitHub, accountLabel, scanLimitNotice, QUICK } from "./setup-quick.js";

const STEPS = ["wizard.stepIdentities", "wizard.stepFolders", "wizard.stepReview"];

const NAME_HINTS = { login: "wizard.nameLogin", global: "wizard.nameGlobal" };

const EMAIL_HINTS = { public: "wizard.emailPublic", noreply: "wizard.emailNoreply", existing: "wizard.emailExisting" };

// wizard holds the user's choices; every change replaces it with a new object
// through setWizard, which also keeps it for this tab, so a reload or the
// browser's Back and Forward do not lose the choices.
const STORE_KEY = "gitpersona-setup";
const STORE_VERSION = 7; // a wizard stored by an older page is not reused
let wizard = loadWizard();
let shownStep = null;
let shownIn = null; // {body, run} of the steps on screen, for Back and Forward

function setWizard(next) {
  wizard = next;
  try {
    if (next) sessionStorage.setItem(STORE_KEY, JSON.stringify(next));
    else sessionStorage.removeItem(STORE_KEY);
  } catch {
    // Storage may be unavailable; the choices then last until a reload.
  }
}

function loadWizard() {
  try {
    const stored = JSON.parse(sessionStorage.getItem(STORE_KEY) ?? "null");
    return stored?.version === STORE_VERSION ? stored : null;
  } catch {
    return null;
  }
}

// remember records a step in the browser history, so Back and Forward move
// between the steps instead of leaving the page.
function remember(step, how = "push") {
  const entry = { setupStep: step };
  if (how === "replace") history.replaceState(entry, "");
  else history.pushState(entry, "");
}

window.addEventListener("popstate", (event) => {
  const step = event.state?.setupStep;
  if (!wizard || !shownIn?.body.isConnected || typeof step !== "number") return;
  update({ step }, shownIn.body, shownIn.run, { fromHistory: true });
});

export function renderSetup(section, { state, run }) {
  if (state.config_exists) {
    setWizard(null);
    section.append(finished(state, run));
    return;
  }
  const body = h("div", {});
  section.append(body);
  if (wizard) {
    show(body, run);
    return;
  }
  replace(body, h("p", { "aria-busy": "true" }, t("wizard.looking")));
  api.proposeSetup().then(({ data }) => {
    setWizard(fromProposal(data));
    remember(QUICK, "replace");
    show(body, run);
  }).catch((err) => replace(body, h("div", { class: "notice fail", role: "alert" }, err.message,
    h("p", {}, t("wizard.signInThenReload")))));
}

function fromProposal(p) {
  // Profiles are told apart by key: two can share an account name on
  // different hosts, and the user may rename their ids.
  const keyOf = (id) => {
    const i = p.profiles.findIndex((x) => x.id === id);
    return i < 0 ? "" : `p${i}`;
  };
  return {
    version: STORE_VERSION,
    step: QUICK,
    defaultKey: keyOf(p.default),
    unmatched: p.unmatched,
    global: p.global,
    ownRules: p.own_rules ?? [],
    shadowed: p.shadowed ?? [],
    profiles: p.profiles.map(({ found = [], ...x }, i) => ({
      ...x, key: `p${i}`, include: true,
      dirs: [...x.dirs.map((d) => ({ dir: d, on: true, why: t("why.existing") })),
        ...found.map((f) => ({ dir: f.dir, on: true, why: f.why }))],
    })),
    pins: p.pins.map((pin) => ({ repo: pin.repo, key: keyOf(pin.profile), why: pin.why, outside: !!pin.outside, on: true })),
    shells: p.shells.map((kind) => ({ kind, on: true })),
    oldRules: { dirs: p.old_rules, kept: p.kept_rules, on: true },
    unassigned: p.unassigned,
    roots: p.roots.join(", "),
    scanned: p.roots.length > 0,
    scanIncomplete: p.scan_incomplete,
    scanUnfinished: p.scan_unfinished ?? [],
    scanTooDeep: p.scan_too_deep,
    githubUnavailable: p.github_unavailable ?? "",
  };
}

// planOf is the configuration the wizard's choices describe.
function planOf(w) {
  const included = w.profiles.filter((p) => p.include);
  const idOf = (key) => included.find((p) => p.key === key)?.id;
  return {
    default: idOf(w.defaultKey) ?? "",
    profiles: included.map(({ id, provider, host, account, name, email, ssh_key, signing, dirs }) =>
      ({ id, provider, host, account, name, email, ssh_key, signing, dirs: dirs.filter((d) => d.on).map((d) => d.dir) })),
    pins: w.pins.filter((pin) => pin.on && idOf(pin.key) && pinNeeded(w, pin))
      .map((pin) => ({ repo: pin.repo, profile: idOf(pin.key) })),
    shells: w.shells.filter((sh) => sh.on).map((sh) => sh.kind),
    // Declining every shell offered is remembered, so doctor does not insist.
    no_shell: w.shells.length > 0 && w.shells.every((sh) => !sh.on),
    // The server removes a rule only while a chosen profile with its email
    // covers its directory.
    remove_old_rules: w.oldRules.on ? w.oldRules.dirs : [],
  };
}

// applyPlan writes the setup; afterwards the wizard is done.
async function applyPlan(run) {
  if (await run(() => api.setupApply(planOf(wizard)), t("wizard.done"))) setWizard(null);
}

function update(changes, body, run, { fromHistory = false } = {}) {
  const moved = changes.step !== undefined && changes.step !== wizard.step;
  if (moved) clearStatus();
  if (moved && !fromHistory) remember(changes.step);
  setWizard({ ...wizard, ...changes });
  show(body, run);
}

function setProfile(index, changes) {
  return wizard.profiles.map((p, i) => (i === index ? { ...p, ...changes } : p));
}

function show(body, run) {
  if (wizard.step === QUICK) {
    replace(body, quickView({
      get: () => wizard, set: setWizard, planOf, update: (changes) => update(changes, body, run),
      apply: () => applyPlan(run), unassigned: () => unassignedCard(chosenProfiles(), body, run),
    }));
    shownIn = { body, run };
    if (shownStep !== wizard.step) body.querySelector("h3")?.focus();
    shownStep = wizard.step;
    return;
  }
  const steps = h("ol", { class: "steps" }, STEPS.map((name, i) =>
    h("li", { "aria-current": i === wizard.step ? "step" : null, class: i < wizard.step ? "done" : null },
      h("span", { class: "visually-hidden" }, i < wizard.step ? t("wizard.stepDone") : ""), t(name))));
  const content = [identities, directories, review][wizard.step](body, run);
  replace(body, steps, content);
  shownIn = { body, run };
  if (shownStep !== wizard.step) body.querySelector("h3")?.focus();
  shownStep = wizard.step;
}

function nav(body, run, { back = true, nextLabel = t("wizard.next") }) {
  return h("div", { class: "actions" },
    back ? h("button", { type: "button", onclick: () => goBack(body, run) }, t("wizard.back")) : null,
    h("button", { type: "submit", class: "primary" }, nextLabel));
}

// goBack follows the browser history when this step was reached through it,
// so the Back button and the browser's Back stay in step.
function goBack(body, run) {
  if (history.state?.setupStep === wizard.step && wizard.step > QUICK) history.back();
  else update({ step: wizard.step - 1 }, body, run);
}

// Step 1: which accounts become profiles, with their names and emails.
function identities(body, run) {
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, t("wizard.identitiesTitle")),
    h("p", { class: "muted" }, t("wizard.identitiesText")),
    wizard.profiles.map((p, i) => identityFields(p, i)),
    defaultChoice(),
    leftOutChoices(() => wizard, setWizard, () => update({}, body, run)),
    nav(body, run, {}));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const included = wizard.profiles.filter((p) => p.include);
    if (included.length === 0) {
      status("fail", t("wizard.chooseOne"));
      document.querySelector("#status .notice")?.focus();
      return;
    }
    const keep = wizard.defaultKey === "" || included.some((p) => p.key === wizard.defaultKey);
    const defaultKey = keep ? wizard.defaultKey : included[0].key;
    update({ step: 1, defaultKey }, body, run);
  });
  return form;
}

function identityFields(p, i) {
  const input = (key, attrs = {}) => {
    const el = h("input", { type: "text", value: p[key], required: true, autocomplete: "off", spellcheck: "false", ...attrs });
    el.addEventListener("change", () => { setWizard({ ...wizard, profiles: setProfile(i, { [key]: el.value.trim() }) }); });
    return el;
  };
  const include = h("input", { type: "checkbox", checked: p.include });
  const isDefault = h("input", { type: "radio", name: "default", value: p.key, checked: p.key === wizard.defaultKey });
  // A fieldset that is disabled is also skipped by form validation, so a
  // profile the user leaves out cannot block Next.
  const fields = h("fieldset", { class: "stack plain", hidden: !p.include, disabled: !p.include },
    field(t("form.id"), input("id", { pattern: "[a-z0-9][a-z0-9\\-]{0,31}", maxlength: 32, title: t("form.idTitle") }), t("wizard.idHint")),
    field(t("form.name"), input("name"), NAME_HINTS[p.name_source] ? t(NAME_HINTS[p.name_source]) : undefined),
    field(t("form.email"), input("email", { type: "email" }), EMAIL_HINTS[p.email_source] ? t(EMAIL_HINTS[p.email_source]) : undefined),
    h("label", { class: "check" }, isDefault, t("wizard.useAsDefault")));
  include.addEventListener("change", () => {
    setWizard({ ...wizard, profiles: setProfile(i, { include: include.checked }) });
    fields.hidden = !include.checked;
    fields.disabled = !include.checked;
  });
  isDefault.addEventListener("change", () => { setWizard({ ...wizard, defaultKey: p.key }); });
  return h("fieldset", {},
    h("legend", {}, accountLabel(p), " ", p.source === "existing" ? badge("info", t("wizard.fromExisting")) : null),
    h("label", { class: "check" }, include, t("wizard.createFor", { account: accountLabel(p) })),
    fields,
    profileNotes(p));
}

// defaultChoice offers no default, and says what commits outside every
// directory use today, so making a profile the default is not a surprise.
function defaultChoice() {
  const none = h("input", { type: "radio", name: "default", value: "", checked: wizard.defaultKey === "" });
  none.addEventListener("change", () => { setWizard({ ...wizard, defaultKey: "" }); });
  const g = wizard.global ?? {};
  const today = g.email ? `${g.name || t("quick.noName")} <${g.email}>` : "";
  return h("fieldset", {},
    h("legend", {}, t("default.title")),
    h("p", { class: "muted" }, today
      ? [t("wizard.todayBefore"), h("span", { class: "mono" }, today), t("wizard.todayAfter")]
      : t("wizard.todayDefault")),
    h("label", { class: "check" }, none,
      today ? t("wizard.noDefaultKeep", { who: today }) : t("wizard.noDefaultGit")),
    wizard.ownRules?.length ? h("p", { class: "notice warn" }, badge("warn"), " ",
      t("wizard.defaultOverrides", { rules: wizard.ownRules.join("; ") })) : null);
}

// Step 2: which directories each profile covers.
function directories(body, run) {
  const included = chosenProfiles();
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, t("wizard.foldersTitle")),
    h("p", { class: "muted" }, t("wizard.foldersText")),
    scanCard(body, run),
    scanLimitNotice(wizard),
    wizard.unassigned.length > 0 ? unassignedCard(included, body, run) : null,
    included.map(([p, i]) => dirFields(p, i, body, run)),
    pinChoices(() => wizard, setWizard),
    oldRulesChoice(() => wizard, setWizard),
    shellChoices(() => wizard, setWizard),
    nav(body, run, {}));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    update({ step: 2 }, body, run);
  });
  return form;
}

// onEnter makes Enter in a field press its own button instead of
// submitting the whole step.
function onEnter(input, button) {
  input.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      button.click();
    }
  });
}

function scanCard(body, run) {
  const roots = h("input", { type: "text", value: wizard.roots, placeholder: "~/code, ~/work", autocomplete: "off", spellcheck: "false" });
  const scan = h("button", { type: "button" }, wizard.scanned ? t("wizard.scanAgain") : t("wizard.findSuggestions"));
  onEnter(roots, scan);
  scan.addEventListener("click", () => busy(scan, async () => {
    try {
      const { data } = await api.discover(roots.value.trim());
      const { profiles, unassigned } = withSuggestions(data.suggestions);
      update({ roots: roots.value.trim(), scanned: true, profiles, unassigned }, body, run);
      status(data.missing.length > 0 ? "warn" : "ok", scanSummary(data));
    } catch (err) {
      status("fail", err.message);
    }
  }));
  return h("div", { class: "card" },
    h("div", { class: "row" },
      field(t("wizard.lookIn"), roots, t("wizard.lookInHint")),
      scan));
}

function scanSummary(data) {
  const n = data.suggestions.length;
  const parts = [];
  if (data.roots.length > 0) {
    parts.push(n > 0 ? tn(n, "wizard.foundOne", "wizard.foundMany", { dirs: data.roots.join(", ") })
      : t("wizard.foundNone", { dirs: data.roots.join(", ") }));
  }
  if (data.missing.length > 0) parts.push(t("wizard.missing", { dirs: data.missing.join(", ") }));
  return parts.join(" ") || t("wizard.noUsualFolders");
}

// withSuggestions adds each suggestion to the profile of its account; those
// owned by someone else, such as an organization, wait for the user to choose.
function withSuggestions(suggestions) {
  const known = (dir) => wizard.profiles.some((p) => p.dirs.some((d) => d.dir === dir)) ||
    wizard.unassigned.some((u) => u.dir === dir);
  const fresh = suggestions.filter((s) => !known(s.dir));
  const why = (s) => (s.repos !== s.total ? t("why.someOf", { repos: s.repos, total: s.total })
    : s.total === 1 ? t("why.one") : t("why.all", { total: s.total }));
  // An account's suggestions go to its first profile: a second profile of
  // the same account (another author) keeps only its own folder.
  const first = (p) => wizard.profiles.find((x) => onGitHub(x) && x.account === p.account) === p;
  const profiles = wizard.profiles.map((p) => {
    const found = fresh.filter((s) => s.account && onGitHub(p) && s.account === p.account && first(p));
    return { ...p, dirs: [...p.dirs, ...found.map((s) => ({ dir: s.dir, on: true, why: why(s) }))] };
  });
  const unassigned = [...wizard.unassigned,
    ...fresh.filter((s) => !profiles.some((p) => onGitHub(p) && p.account === s.account)).map((s) => ({ dir: s.dir, owner: s.owner, why: why(s) }))];
  return { profiles, unassigned };
}

function unassignedCard(included, body, run) {
  return h("div", { class: "card" },
    h("h4", {}, t("wizard.ownedElsewhere")),
    h("p", { class: "muted" }, t("wizard.ownedElsewhereText")),
    wizard.unassigned.map((u, k) => {
      const select = h("select", {},
        h("option", { value: "" }, t("quick.leaveOut")),
        included.map(([p, i]) => h("option", { value: String(i) }, `${p.id} (${accountLabel(p)})`)));
      select.addEventListener("change", () => {
        if (select.value === "") return;
        const i = Number(select.value);
        const dirs = [...wizard.profiles[i].dirs, { dir: u.dir, on: true, why: t("why.ownedBy", { why: u.why, owner: u.owner }) }];
        update({ profiles: setProfile(i, { dirs }), unassigned: wizard.unassigned.filter((_, j) => j !== k) }, body, run);
        // The step is drawn again; keep focus on the directory just added.
        const added = [...body.querySelectorAll("label.check")].find((l) => l.textContent.startsWith(u.dir + " ·"));
        (added?.querySelector("input") ?? body.querySelector("select") ?? body.querySelector("button.primary"))?.focus();
      });
      return field(`${u.dir} · ${u.owner} · ${u.why}`, select);
    }));
}

// cleanDir writes a directory without a trailing slash.
function cleanDir(dir) {
  return dir.length > 1 ? dir.replace(/\/+$/, "") : dir;
}

function dirFields(p, i, body, run) {
  const add = h("input", { type: "text", autocomplete: "off", spellcheck: "false", placeholder: "~/code/" + p.id });
  const addButton = h("button", { type: "button" }, t("shells.addButton"));
  onEnter(add, addButton);
  addButton.addEventListener("click", () => busy(addButton, async () => {
    const typed = cleanDir(add.value.trim());
    if (!typed) return;
    if (!typed.startsWith("~/") && !typed.startsWith("/")) {
      status("fail", t("wizard.pathFormat"));
      add.focus();
      return;
    }
    const dir = await normalizePath(typed);
    const owner = wizard.profiles.find((x) => x.include && x.dirs.some((d) => d.on && cleanDir(d.dir) === dir));
    if (owner) {
      status("fail", owner === wizard.profiles[i] ? t("wizard.alreadyListed", { dir })
        : t("wizard.alreadyChosen", { dir, id: owner.id }));
      add.focus();
      return;
    }
    clearStatus();
    const dirs = [...wizard.profiles[i].dirs, { dir, on: true, why: t("why.byYou") }];
    update({ profiles: setProfile(i, { dirs }) }, body, run);
    body.querySelectorAll("fieldset input[type=text]")[included(i)]?.focus();
  }));
  return h("fieldset", {},
    h("legend", {}, `${p.id} (${accountLabel(p)})`),
    p.dirs.length === 0 ? h("p", { class: "empty" }, t("wizard.noFolders")) : null,
    p.dirs.map((d, j) => {
      const box = h("input", { type: "checkbox", checked: d.on });
      box.addEventListener("change", () => {
        const dirs = wizard.profiles[i].dirs.map((x, k) => (k === j ? { ...x, on: box.checked } : x));
        setWizard({ ...wizard, profiles: setProfile(i, { dirs }) });
      });
      return h("label", { class: "check" }, box, h("span", { class: "mono" }, d.dir), h("span", { class: "muted" }, ` · ${d.why}`));
    }),
    h("div", { class: "row" }, field(t("accounts.addFolder"), add), addButton));
}

// included returns the position of profile i among the included profiles.
function included(i) {
  return wizard.profiles.slice(0, i).filter((p) => p.include).length;
}

// chosenProfiles returns the included profiles with their positions.
function chosenProfiles() {
  return wizard.profiles.map((p, i) => [p, i]).filter(([p]) => p.include);
}

// Step 3: every file the setup writes, then apply.
function review(body, run) {
  const plan = planOf(wizard);
  const result = h("div", { "aria-busy": "true" }, h("p", {}, t("wizard.preparing")));
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, t("wizard.reviewTitle")),
    h("p", { class: "muted" }, t("wizard.reviewText")),
    result,
    nav(body, run, { nextLabel: t("apply.apply") }));
  const apply = form.querySelector("button.primary");
  apply.disabled = true;
  api.setupPreview(plan).then(({ data }) => {
    replace(result, data.map((c, i) => h("details", { class: "change", open: i === 0 },
      h("summary", {}, badge(c.action === "create" ? "info" : "warn", c.action === "create" ? t("file.new") : t("file.changed")),
        h("span", { class: "mono" }, c.path)),
      diffBlock(c.diff))));
    apply.disabled = false;
  }).catch((err) => {
    replace(result, h("div", { class: "notice fail", role: "alert" }, err.message,
      h("p", {}, t("wizard.goBack"))));
  }).finally(() => result.removeAttribute("aria-busy"));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    busy(apply, async () => {
      await applyPlan(run);
    });
  });
  return form;
}

function finished(state, run) {
  if (state.config_error) return h("div", {}, configProblem(state));
  return h("div", {},
    h("div", { class: "notice ok" }, badge("ok", t("wizard.setUp")), " ", t("wizard.inPlace")),
    doctorCard(state.doctor, run),
    h("div", { class: "card" },
      h("h3", {}, t("wizard.nextSteps")),
      h("ul", {},
        // Only while the default shell does not load GitPersona yet.
        state.shells?.installed?.some((sh) => sh.kind === state.shells.default?.kind && !sh.integrated)
          ? h("li", {}, h("a", { href: "#shells" }, t("wizard.loadShells")), t("wizard.loadShellsAfter")) : null,
        h("li", {}, h("a", { href: "#overview" }, t("wizard.checkIdentity")), t("wizard.checkIdentityAfter")))));
}
