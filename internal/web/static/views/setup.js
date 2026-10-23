// First-run setup in three steps: confirm identities, confirm directories,
// then review every file, apply and check. Nothing is written before the
// last step.

import { api, normalizePath } from "../api.js";
import { h, replace, badge, field, busy, status, clearStatus } from "../dom.js";
import { doctorCard, configProblem } from "./overview.js";
import { diffBlock } from "./apply.js";
import { quickView, pinChoices, shellChoices, oldRulesChoice, leftOutChoices, profileNotes, pinNeeded, scanLimitNotice, QUICK } from "./setup-quick.js";

const STEPS = ["Identities", "Directories", "Review & apply"];

const NAME_HINTS = {
  login: "This account shows no public name, so this is its login",
  global: "Your includeIf setup sets no name, so this is your global Git name",
};

const EMAIL_HINTS = {
  public: "The public email on this account's GitHub profile",
  noreply: "This account shows no public email, so this is its GitHub noreply address",
  existing: "The email your Git configuration already uses for this account",
};

// wizard holds the user's choices; every change replaces it with a new object
// through setWizard, which also keeps it for this tab, so a reload or the
// browser's Back and Forward do not lose the choices.
const STORE_KEY = "gitpersona-setup";
const STORE_VERSION = 6; // a wizard stored by an older page is not reused
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
  replace(body, h("p", { "aria-busy": "true" }, "Looking at your accounts, Git settings and repositories…"));
  api.proposeSetup().then(({ data }) => {
    setWizard(fromProposal(data));
    remember(QUICK, "replace");
    show(body, run);
  }).catch((err) => replace(body, h("div", { class: "notice fail", role: "alert" }, err.message,
    h("p", {}, "Sign in with `gh auth login` for each account, then reload this page."))));
}

function fromProposal(p) {
  const accountOf = (id) => p.profiles.find((x) => x.id === id)?.account;
  return {
    version: STORE_VERSION,
    step: QUICK,
    defaultAccount: accountOf(p.default) ?? "",
    unmatched: p.unmatched,
    global: p.global,
    ownRules: p.own_rules ?? [],
    shadowed: p.shadowed ?? [],
    profiles: p.profiles.map(({ found = [], ...x }) => ({
      ...x, include: true,
      dirs: [...x.dirs.map((d) => ({ dir: d, on: true, why: "your existing setup" })),
        ...found.map((f) => ({ dir: f.dir, on: true, why: f.why }))],
    })),
    pins: p.pins.map((pin) => ({ repo: pin.repo, account: accountOf(pin.profile), why: pin.why, outside: !!pin.outside, on: true })),
    shells: p.shells.map((kind) => ({ kind, on: true })),
    oldRules: { dirs: p.old_rules, kept: p.kept_rules, on: true },
    unassigned: p.unassigned,
    roots: p.roots.join(", "),
    scanned: p.roots.length > 0,
    scanIncomplete: p.scan_incomplete,
    scanUnfinished: p.scan_unfinished ?? [],
    scanTooDeep: p.scan_too_deep,
  };
}

// planOf is the configuration the wizard's choices describe.
function planOf(w) {
  const included = w.profiles.filter((p) => p.include);
  const idOf = (account) => included.find((p) => p.account === account)?.id;
  return {
    default: idOf(w.defaultAccount) ?? "",
    profiles: included.map(({ id, account, name, email, ssh_key, signing, dirs }) =>
      ({ id, account, name, email, ssh_key, signing, dirs: dirs.filter((d) => d.on).map((d) => d.dir) })),
    pins: w.pins.filter((pin) => pin.on && idOf(pin.account) && pinNeeded(w, pin))
      .map((pin) => ({ repo: pin.repo, profile: idOf(pin.account) })),
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
  if (await run(() => api.setupApply(planOf(wizard)), "GitPersona is set up. Git now uses your profiles.")) setWizard(null);
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
      h("span", { class: "visually-hidden" }, i < wizard.step ? "Done: " : ""), name)));
  const content = [identities, directories, review][wizard.step](body, run);
  replace(body, steps, content);
  shownIn = { body, run };
  if (shownStep !== wizard.step) body.querySelector("h3")?.focus();
  shownStep = wizard.step;
}

function nav(body, run, { back = true, nextLabel = "Next" }) {
  return h("div", { class: "actions" },
    back ? h("button", { type: "button", onclick: () => goBack(body, run) }, "Back") : null,
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
    h("h3", { tabindex: "-1" }, "1. Confirm your identities"),
    h("p", { class: "muted" }, "One profile for each account signed in to gh, with the name and email shown on its GitHub profile. " +
      "Change them here if your commits should use something else."),
    wizard.profiles.map((p, i) => identityFields(p, i)),
    defaultChoice(),
    leftOutChoices(() => wizard, setWizard, () => update({}, body, run)),
    nav(body, run, {}));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const included = wizard.profiles.filter((p) => p.include);
    if (included.length === 0) {
      status("fail", "Choose at least one account.");
      document.querySelector("#status .notice")?.focus();
      return;
    }
    const keep = wizard.defaultAccount === "" || included.some((p) => p.account === wizard.defaultAccount);
    const defaultAccount = keep ? wizard.defaultAccount : included[0].account;
    update({ step: 1, defaultAccount }, body, run);
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
  const isDefault = h("input", { type: "radio", name: "default", value: p.account, checked: p.account === wizard.defaultAccount });
  // A fieldset that is disabled is also skipped by form validation, so a
  // profile the user leaves out cannot block Next.
  const fields = h("fieldset", { class: "stack plain", hidden: !p.include, disabled: !p.include },
    field("Profile name", input("id", { pattern: "[a-z0-9][a-z0-9\\-]{0,31}", maxlength: 32, title: "Lowercase letters, digits and -, starting with a letter or digit" }), "Lowercase letters, digits and -"),
    field("Author name", input("name"), NAME_HINTS[p.name_source]),
    field("Author email", input("email", { type: "email" }), EMAIL_HINTS[p.email_source]),
    h("label", { class: "check" }, isDefault, "Use this profile where no directory rule matches (default)"));
  include.addEventListener("change", () => {
    setWizard({ ...wizard, profiles: setProfile(i, { include: include.checked }) });
    fields.hidden = !include.checked;
    fields.disabled = !include.checked;
  });
  isDefault.addEventListener("change", () => { setWizard({ ...wizard, defaultAccount: p.account }); });
  return h("fieldset", {},
    h("legend", {}, p.account, " ", p.source === "existing" ? badge("info", "From your existing setup") : null),
    h("label", { class: "check" }, include, `Create a profile for ${p.account}`),
    fields,
    profileNotes(p));
}

// defaultChoice offers no default, and says what commits outside every
// directory use today, so making a profile the default is not a surprise.
function defaultChoice() {
  const none = h("input", { type: "radio", name: "default", value: "", checked: wizard.defaultAccount === "" });
  none.addEventListener("change", () => { setWizard({ ...wizard, defaultAccount: "" }); });
  const g = wizard.global ?? {};
  const today = g.email ? `${g.name || "(no name)"} <${g.email}>` : "";
  return h("fieldset", {},
    h("legend", {}, "Outside your directories"),
    h("p", { class: "muted" }, today
      ? ["Commits outside every directory use ", h("span", { class: "mono" }, today),
        " today (your global Git settings). The default profile replaces it."]
      : "Commits outside every directory use the default profile chosen above."),
    h("label", { class: "check" }, none,
      today ? `No default: keep ${today}` : "No default: keep Git's own settings"),
    wizard.ownRules?.length ? h("p", { class: "notice warn" }, badge("warn"),
      ` A default profile overrides your own includeIf rules everywhere: ${wizard.ownRules.join("; ")}.`) : null);
}

// Step 2: which directories each profile covers.
function directories(body, run) {
  const included = chosenProfiles();
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, "2. Choose directories"),
    h("p", { class: "muted" }, "Every repository under a chosen directory uses that profile. You can change this later."),
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
  const scan = h("button", { type: "button" }, wizard.scanned ? "Scan again" : "Find suggestions");
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
      field("Folders to look in (optional)", roots,
        "GitPersona reads only the remote address of each repository; empty looks in common places such as ~/code"),
      scan));
}

function scanSummary(data) {
  const n = data.suggestions.length;
  const parts = [];
  if (data.roots.length > 0) {
    parts.push(n > 0 ? `Found ${n} suggestion${n === 1 ? "" : "s"} in ${data.roots.join(", ")}.`
      : `No folder in ${data.roots.join(", ")} has repositories that mostly belong to one owner.`);
  }
  if (data.missing.length > 0) parts.push(`These folders do not exist: ${data.missing.join(", ")}.`);
  return parts.join(" ") || "None of the usual code folders exist; type the folders to look in.";
}

// withSuggestions adds each suggestion to the profile of its account; those
// owned by someone else, such as an organization, wait for the user to choose.
function withSuggestions(suggestions) {
  const known = (dir) => wizard.profiles.some((p) => p.dirs.some((d) => d.dir === dir)) ||
    wizard.unassigned.some((u) => u.dir === dir);
  const fresh = suggestions.filter((s) => !known(s.dir));
  const why = (s) => (s.repos !== s.total ? `${s.repos} of ${s.total} repositories there`
    : s.total === 1 ? "its one repository" : `all ${s.total} repositories there`);
  const profiles = wizard.profiles.map((p) => {
    const found = fresh.filter((s) => s.account && s.account === p.account);
    return { ...p, dirs: [...p.dirs, ...found.map((s) => ({ dir: s.dir, on: true, why: why(s) }))] };
  });
  const unassigned = [...wizard.unassigned,
    ...fresh.filter((s) => !profiles.some((p) => p.account === s.account)).map((s) => ({ dir: s.dir, owner: s.owner, why: why(s) }))];
  return { profiles, unassigned };
}

function unassignedCard(included, body, run) {
  return h("div", { class: "card" },
    h("h4", {}, "Owned by someone else"),
    h("p", { class: "muted" }, "Most repositories in these folders belong to an owner that is not one of your accounts, " +
      "often an organization. Choose the profile to use there, or leave them out."),
    wizard.unassigned.map((u, k) => {
      const select = h("select", {},
        h("option", { value: "" }, "Leave out"),
        included.map(([p, i]) => h("option", { value: String(i) }, `${p.id} (${p.account})`)));
      select.addEventListener("change", () => {
        if (select.value === "") return;
        const i = Number(select.value);
        const dirs = [...wizard.profiles[i].dirs, { dir: u.dir, on: true, why: `${u.why}, owned by ${u.owner}` }];
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
  const addButton = h("button", { type: "button" }, "Add");
  onEnter(add, addButton);
  addButton.addEventListener("click", () => busy(addButton, async () => {
    const typed = cleanDir(add.value.trim());
    if (!typed) return;
    if (!typed.startsWith("~/") && !typed.startsWith("/")) {
      status("fail", "Write the directory as an absolute path or one starting with ~/, for example ~/code/work.");
      add.focus();
      return;
    }
    const dir = await normalizePath(typed);
    const owner = wizard.profiles.find((x) => x.include && x.dirs.some((d) => d.on && cleanDir(d.dir) === dir));
    if (owner) {
      status("fail", owner === wizard.profiles[i] ? `${dir} is already in the list.`
        : `${dir} is already chosen for ${owner.id}; untick it there first.`);
      add.focus();
      return;
    }
    clearStatus();
    const dirs = [...wizard.profiles[i].dirs, { dir, on: true, why: "added by you" }];
    update({ profiles: setProfile(i, { dirs }) }, body, run);
    body.querySelectorAll("fieldset input[type=text]")[included(i)]?.focus();
  }));
  return h("fieldset", {},
    h("legend", {}, `${p.id} (${p.account})`),
    p.dirs.length === 0 ? h("p", { class: "empty" }, "No directories yet.") : null,
    p.dirs.map((d, j) => {
      const box = h("input", { type: "checkbox", checked: d.on });
      box.addEventListener("change", () => {
        const dirs = wizard.profiles[i].dirs.map((x, k) => (k === j ? { ...x, on: box.checked } : x));
        setWizard({ ...wizard, profiles: setProfile(i, { dirs }) });
      });
      return h("label", { class: "check" }, box, h("span", { class: "mono" }, d.dir), h("span", { class: "muted" }, ` · ${d.why}`));
    }),
    h("div", { class: "row" }, field("Add a directory", add), addButton));
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
  const result = h("div", { "aria-busy": "true" }, h("p", {}, "Preparing the files…"));
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, "3. Review and apply"),
    h("p", { class: "muted" }, "These files will be written. Existing files are backed up first, so you can undo this under Backups."),
    result,
    nav(body, run, { nextLabel: "Apply" }));
  const apply = form.querySelector("button.primary");
  apply.disabled = true;
  api.setupPreview(plan).then(({ data }) => {
    replace(result, data.map((c, i) => h("details", { class: "change", open: i === 0 },
      h("summary", {}, badge(c.action === "create" ? "info" : "warn", c.action === "create" ? "New file" : "Changed"),
        h("span", { class: "mono" }, c.path)),
      diffBlock(c.diff))));
    apply.disabled = false;
  }).catch((err) => {
    replace(result, h("div", { class: "notice fail", role: "alert" }, err.message,
      h("p", {}, "Go back to fix it.")));
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
    h("div", { class: "notice ok" }, badge("ok", "Set up"),
      " Your configuration is in place. Change it any time under Profiles."),
    doctorCard(state.doctor, run),
    h("div", { class: "card" },
      h("h3", {}, "Next"),
      h("ul", {},
        // Only while the default shell does not load GitPersona yet.
        state.shells?.installed?.some((sh) => sh.kind === state.shells.default?.kind && !sh.integrated)
          ? h("li", {}, h("a", { href: "#shells" }, "Load GitPersona in your shells"), " for gh switching and the prompt.") : null,
        h("li", {}, h("a", { href: "#overview" }, "Check which identity applies"), " to any directory."))));
}
