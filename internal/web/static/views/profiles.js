// Profiles: one card per profile with its directories and pinned
// repositories, the default profile, and a form for a new profile.
// Every change is saved, applied and backed up at once.

import { api, normalizePath } from "../api.js";
import { h, replace, badge, field, busy, confirm } from "../dom.js";
import { factList, configProblem } from "./overview.js";
import { profileForm } from "./profile-form.js";

const SIGNING_LABELS = { ssh: "SSH", gpg: "GPG", none: "off" };

export function renderProfiles(section, ctx) {
  const { state } = ctx;
  if (state.config_error) {
    section.append(configProblem(state));
    // A file that does not parse has no profiles to show or change.
    if (state.config.profiles.length === 0) return;
  }
  const profiles = state.config.profiles;
  section.append(
    h("p", { class: "muted" }, "Changes here are saved and applied right away. Each one is backed up, so you can undo it under Backups."),
    profiles.length > 0 ? defaultCard(ctx) : null,
    profiles.length > 0
      ? h("div", { class: "grid" }, profiles.map((p) => profileCard(p, ctx)))
      : h("p", { class: "empty" }, "No profiles yet."),
    h("div", { class: "card" }, h("h3", {}, "Add a profile"),
      profileForm({
        accounts: state.accounts,
        onSubmit: (profile) => ctx.run(() => api.addProfile(profile), `Added profile ${profile.id}.`),
      })));
}

function defaultCard({ state, run }) {
  const select = h("select", { name: "default" },
    h("option", { value: "" }, "None: outside any rule, Git's own settings apply"),
    state.config.profiles.map((p) => h("option", { value: p.id, selected: p.id === state.config.default }, p.id)));
  const save = h("button", { type: "submit" }, "Save");
  const form = h("form", { class: "row" },
    field("Default profile", select, "Used in directories that no rule covers"), save);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    busy(save, () => run(() => api.setDefault(select.value),
      select.value ? `Default profile is now ${select.value}.` : "Removed the default profile."));
  });
  return h("div", { class: "card" }, form);
}

function profileCard(p, ctx) {
  const { state, run } = ctx;
  const card = h("article", { class: "card", "aria-labelledby": `profile-${p.id}` });
  const pins = state.config.pins.filter((pin) => pin.profile === p.id);

  const show = () => replace(card,
    h("div", { class: "card-head" },
      h("h3", { id: `profile-${p.id}` }, p.id),
      p.id === state.config.default ? badge("info", "Default") : null,
      accountBadge(p, state)),
    factList([
      ["Account", p.host && p.host !== "github.com" ? `${p.account} on ${p.host}` : p.account],
      ["Author", `${p.name} <${p.email}>`],
      ["SSH key", p.ssh_key],
      ["Signing", p.signing?.method ? [SIGNING_LABELS[p.signing.method] ?? p.signing.method, p.signing.key].filter(Boolean).join(" · ") : ""],
    ]),
    pathList({
      title: "Directories", items: p.dirs, empty: "No directories yet.",
      removeLabel: (dir) => `Remove ${dir}`,
      onRemove: async (dir) => await confirm("Remove this directory?",
        `Repositories under ${dir} will no longer use ${p.id}.`, "Remove") &&
        run(() => api.removeDir(dir), `Removed ${dir}.`),
    }),
    addPathForm({
      label: "Add a directory", hint: `Every repository under it uses this profile, e.g. ~/code/${p.id}`,
      button: "Add directory",
      onAdd: async (typed) => {
        const dir = await normalizePath(typed);
        const owner = ownerOf(state, dir);
        if (owner && owner !== p.id && !(await confirm(`Move ${dir} to ${p.id}?`,
          `${dir} uses ${owner} now. Moving it makes its repositories use ${p.id} instead.`, "Move"))) {
          return;
        }
        await run(() => api.addDir(dir, p.id), null); // the log says what changed
      },
    }),
    pathList({
      title: "Pinned repositories", items: pins.map((pin) => pin.repo), empty: "None. Pin a repository that sits under another profile's directory.",
      removeLabel: (repo) => `Unpin ${repo}`,
      onRemove: async (repo) => await confirm("Unpin this repository?",
        `${repo} will follow the directory rules again.`, "Unpin") &&
        run(() => api.removePin(repo), `Unpinned ${repo}.`),
    }),
    addPathForm({
      label: "Pin a repository", hint: "The repository's top directory", button: "Pin",
      onAdd: async (typed) => {
        const repo = await normalizePath(typed);
        await run(() => api.addPin(repo, p.id), `${repo} now uses ${p.id}.`);
      },
    }),
    h("div", { class: "actions" },
      h("button", { type: "button", onclick: edit, "aria-label": `Edit ${p.id}` }, "Edit"),
      h("button", { type: "button", class: "danger", onclick: remove, "aria-label": `Remove profile ${p.id}` }, "Remove profile")));

  function edit() {
    replace(card, h("h3", { id: `profile-${p.id}` }, `Edit ${p.id}`),
      profileForm({
        profile: p, accounts: state.accounts, onCancel: show,
        onSubmit: (changes) => run(() => api.updateProfile(p.id, changes), `Saved ${p.id}.`),
      }));
    card.querySelector("input")?.focus();
  }

  async function remove() {
    const extra = [p.dirs.length ? `its ${p.dirs.length} director${p.dirs.length === 1 ? "y" : "ies"}` : "",
      pins.length ? `${pins.length} pin${pins.length === 1 ? "" : "s"}` : ""].filter(Boolean).join(" and ");
    const fallback = p.id === state.config.default
      ? ` ${p.id} is your default profile, so directories without a rule will use Git's own identity until you choose another default.`
      : "";
    if (await confirm(`Remove ${p.id}?`, `This removes the profile${extra ? ` with ${extra}` : ""}.${fallback} You can undo it under Backups.`, "Remove profile")) {
      await run(() => api.removeProfile(p.id), `Removed profile ${p.id}.`);
    }
  }

  show();
  return card;
}

function accountBadge(p, state) {
  if (p.host && p.host !== "github.com") return null;
  const account = state.accounts.find((a) => a.login === p.account);
  if (account?.state === "success") return badge("ok", "Signed in");
  return badge("warn", "Not signed in to gh");
}

function pathList({ title, items, empty, removeLabel, onRemove }) {
  return [
    h("h4", {}, title),
    items.length === 0
      ? h("p", { class: "empty" }, empty)
      : h("ul", { class: "rows" }, items.map((item) => {
        const button = h("button", { type: "button", class: "small danger", "aria-label": removeLabel(item) }, "Remove");
        button.addEventListener("click", () => busy(button, () => onRemove(item)));
        return h("li", {}, h("span", { class: "mono" }, item), button);
      })),
  ];
}

function addPathForm({ label, hint, button, onAdd }) {
  const input = h("input", { type: "text", required: true, autocomplete: "off", spellcheck: "false" });
  const submit = h("button", { type: "submit" }, button);
  const form = h("form", { class: "row" }, field(label, input, hint), submit);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    busy(submit, () => onAdd(input.value.trim()));
  });
  return form;
}

// ownerOf returns the profile whose directories include dir (ignoring a
// trailing slash), or "".
function ownerOf(state, dir) {
  const clean = (d) => (d.length > 1 ? d.replace(/\/+$/, "") : d);
  return state.config.profiles.find((p) => p.dirs.some((d) => clean(d) === clean(dir)))?.id ?? "";
}
