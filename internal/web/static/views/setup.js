// First-run setup in three steps: confirm identities, confirm directories,
// then review every file, apply and check. Nothing is written before the
// last step.

import { api } from "../api.js";
import { h, replace, badge, field, busy, status, clearStatus } from "../dom.js";
import { doctorCard } from "./overview.js";
import { diffBlock } from "./apply.js";

const STEPS = ["Identities", "Directories", "Review & apply"];

const EMAIL_HINTS = {
  public: "The public email on this account's GitHub profile",
  noreply: "This account shows no public email, so this is its GitHub noreply address",
  existing: "The email your Git configuration already uses for this account",
};

// wizard holds the user's choices; every change replaces it with a new object.
let wizard = null;
let shownStep = null;

export function renderSetup(section, { state, run }) {
  if (state.config_exists) {
    section.append(finished(state));
    return;
  }
  const body = h("div", {});
  section.append(body);
  if (wizard) {
    show(body, run);
    return;
  }
  replace(body, h("p", { "aria-busy": "true" }, "Asking gh which accounts are signed in…"));
  api.proposeSetup().then(({ data }) => {
    wizard = fromProposal(data);
    show(body, run);
  }).catch((err) => replace(body, h("div", { class: "notice fail", role: "alert" }, err.message,
    h("p", {}, "Sign in with `gh auth login` for each account, then reload this page."))));
}

function fromProposal(p) {
  return {
    step: 0,
    defaultAccount: p.profiles.find((x) => x.id === p.default)?.account ?? "",
    unmatched: p.unmatched,
    profiles: p.profiles.map((x) => ({ ...x, include: true, dirs: x.dirs.map((d) => ({ dir: d, on: true, why: "your existing setup" })) })),
    unassigned: [],
    roots: "",
    scanned: false,
  };
}

function update(changes, body, run) {
  if (changes.step !== undefined && changes.step !== wizard.step) clearStatus();
  wizard = { ...wizard, ...changes };
  show(body, run);
}

function setProfile(index, changes) {
  return wizard.profiles.map((p, i) => (i === index ? { ...p, ...changes } : p));
}

function show(body, run) {
  const steps = h("ol", { class: "steps" }, STEPS.map((name, i) =>
    h("li", { "aria-current": i === wizard.step ? "step" : null, class: i < wizard.step ? "done" : null },
      h("span", { class: "visually-hidden" }, i < wizard.step ? "Done: " : ""), name)));
  const content = [identities, directories, review][wizard.step](body, run);
  replace(body, steps, content);
  if (shownStep !== wizard.step) body.querySelector("h3")?.focus();
  shownStep = wizard.step;
}

function nav(body, run, { back = true, nextLabel = "Next" }) {
  return h("div", { class: "actions" },
    back ? h("button", { type: "button", onclick: () => update({ step: wizard.step - 1 }, body, run) }, "Back") : null,
    h("button", { type: "submit", class: "primary" }, nextLabel));
}

// Step 1: which accounts become profiles, with their names and emails.
function identities(body, run) {
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, "1. Confirm your identities"),
    h("p", { class: "muted" }, "One profile for each account signed in to gh, with the name and email shown on its GitHub profile. " +
      "Change them here if your commits should use something else."),
    wizard.profiles.map((p, i) => identityFields(p, i)),
    wizard.unmatched.length > 0 ? unmatchedNote() : null,
    nav(body, run, { back: false }));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const included = wizard.profiles.filter((p) => p.include);
    if (included.length === 0) {
      status("fail", "Choose at least one account.");
      return;
    }
    const defaultAccount = included.some((p) => p.account === wizard.defaultAccount) ? wizard.defaultAccount : included[0].account;
    update({ step: 1, defaultAccount }, body, run);
  });
  return form;
}

function identityFields(p, i) {
  const input = (key, attrs = {}) => {
    const el = h("input", { type: "text", value: p[key], required: true, autocomplete: "off", spellcheck: "false", ...attrs });
    el.addEventListener("change", () => { wizard = { ...wizard, profiles: setProfile(i, { [key]: el.value.trim() }) }; });
    return el;
  };
  const include = h("input", { type: "checkbox", checked: p.include });
  const isDefault = h("input", { type: "radio", name: "default", value: p.account, checked: p.account === wizard.defaultAccount });
  // A fieldset that is disabled is also skipped by form validation, so a
  // profile the user leaves out cannot block Next.
  const fields = h("fieldset", { class: "stack plain", hidden: !p.include, disabled: !p.include },
    field("Profile name", input("id", { pattern: "[a-z0-9][a-z0-9\\-]{0,31}", maxlength: 32, title: "Lowercase letters, digits and -, starting with a letter or digit" }), "Lowercase letters, digits and -"),
    field("Author name", input("name")),
    field("Author email", input("email", { type: "email" }), EMAIL_HINTS[p.email_source]),
    h("label", { class: "check" }, isDefault, "Use this profile where no directory rule matches (default)"));
  include.addEventListener("change", () => {
    wizard = { ...wizard, profiles: setProfile(i, { include: include.checked }) };
    fields.hidden = !include.checked;
    fields.disabled = !include.checked;
  });
  isDefault.addEventListener("change", () => { wizard = { ...wizard, defaultAccount: p.account }; });
  return h("fieldset", {},
    h("legend", {}, p.account, " ", p.source === "existing" ? badge("info", "From your existing setup") : null),
    h("label", { class: "check" }, include, `Create a profile for ${p.account}`),
    fields);
}

function unmatchedNote() {
  return h("div", { class: "notice warn" },
    h("p", {}, badge("warn"), " These identities in your Git configuration belong to no account signed in to gh, so they are not included:"),
    h("ul", {}, wizard.unmatched.map((u) => h("li", {}, `${u.name} <${u.email}> · ${u.dirs.join(", ")} (${u.source})`))),
    h("p", {}, "Sign in to that account with `gh auth login` and reload this page to include it."));
}

// Step 2: which directories each profile covers.
function directories(body, run) {
  const included = wizard.profiles.map((p, i) => [p, i]).filter(([p]) => p.include);
  const form = h("form", { class: "stack" },
    h("h3", { tabindex: "-1" }, "2. Choose directories"),
    h("p", { class: "muted" }, "Every repository under a chosen directory uses that profile. You can change this later."),
    scanCard(body, run),
    wizard.unassigned.length > 0 ? unassignedCard(included, body, run) : null,
    included.map(([p, i]) => dirFields(p, i, body, run)),
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
  const why = (s) => `${s.repos} of ${s.total} repositories there`;
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
  addButton.addEventListener("click", () => {
    const dir = cleanDir(add.value.trim());
    if (!dir) return;
    if (!dir.startsWith("~/") && !dir.startsWith("/")) {
      status("fail", "Write the directory as an absolute path or one starting with ~/, for example ~/code/work.");
      add.focus();
      return;
    }
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
  });
  return h("fieldset", {},
    h("legend", {}, `${p.id} (${p.account})`),
    p.dirs.length === 0 ? h("p", { class: "empty" }, "No directories yet.") : null,
    p.dirs.map((d, j) => {
      const box = h("input", { type: "checkbox", checked: d.on });
      box.addEventListener("change", () => {
        const dirs = wizard.profiles[i].dirs.map((x, k) => (k === j ? { ...x, on: box.checked } : x));
        wizard = { ...wizard, profiles: setProfile(i, { dirs }) };
      });
      return h("label", { class: "check" }, box, h("span", { class: "mono" }, d.dir), h("span", { class: "muted" }, ` · ${d.why}`));
    }),
    h("div", { class: "row" }, field("Add a directory", add), addButton));
}

// included returns the position of profile i among the included profiles.
function included(i) {
  return wizard.profiles.slice(0, i).filter((p) => p.include).length;
}

// Step 3: every file the setup writes, then apply.
function review(body, run) {
  const plan = {
    default: wizard.profiles.find((p) => p.include && p.account === wizard.defaultAccount)?.id ?? "",
    profiles: wizard.profiles.filter((p) => p.include).map(({ id, account, name, email, dirs }) =>
      ({ id, account, name, email, dirs: dirs.filter((d) => d.on).map((d) => d.dir) })),
  };
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
      if (await run(() => api.setupApply(plan), "GitPersona is set up. Git now uses your profiles.")) wizard = null;
    });
  });
  return form;
}

function finished(state) {
  return h("div", {},
    h("div", { class: "notice ok" }, badge("ok", "Set up"),
      " Your configuration is in place. Change it any time under Profiles."),
    doctorCard(state.doctor),
    h("div", { class: "card" },
      h("h3", {}, "Next"),
      h("ul", {},
        h("li", {}, h("a", { href: "#shells" }, "Load GitPersona in your shells"), " for gh switching and the prompt."),
        h("li", {}, h("a", { href: "#overview" }, "Check which identity applies"), " to any directory."))));
}
