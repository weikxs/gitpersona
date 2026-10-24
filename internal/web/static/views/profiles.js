// Accounts: one card per account, and under it each identity (profile) that
// pushes as that account, with the folders and pinned repositories it covers.
// Every change is saved, applied and backed up at once.

import { api, normalizePath } from "../api.js";
import { h, replace, badge, field, busy, confirm } from "../dom.js";
import { t, tn } from "../i18n.js";
import { icon } from "../icons.js";
import { factList, configProblem, forgetVerification } from "./overview.js";
import { profileForm } from "./profile-form.js";
import { groupByAccount, accountLabel, signedIn, avatar, authorDiffers } from "./accounts.js";

const SIGNING_LABELS = { ssh: "signing.ssh", gpg: "signing.gpg", none: "signing.off" };

export function renderProfiles(section, ctx) {
  const { state } = ctx;
  if (state.config_error) {
    section.append(configProblem(state));
    // A file that does not parse has no profiles to show or change.
    if (state.config.profiles.length === 0) return;
  }
  // Changes here change who commits and pushes where: check again later.
  const run = (action, success) => {
    forgetVerification();
    return ctx.run(action, success);
  };
  const groups = groupByAccount(state.config);
  section.append(
    h("p", { class: "lead" }, t("accounts.lead")),
    groups.length > 0
      ? h("div", { class: "accounts" }, groups.map((g) => accountCard(g, { ...ctx, run })))
      : h("p", { class: "empty" }, t("accounts.none")),
    groups.length > 0 ? defaultCard({ ...ctx, run }) : null,
    h("div", { class: "card" }, h("h3", {}, t("accounts.add")),
      h("p", { class: "muted" }, t("accounts.addHint")),
      profileForm({
        accounts: state.accounts,
        onSubmit: (profile) => run(() => api.addProfile(profile), t("accounts.added", { id: profile.id })),
      })));
}

function accountCard(group, ctx) {
  const { state } = ctx;
  const login = signedIn(group, state);
  return h("article", { class: "card persona", "aria-label": accountLabel(group) },
    h("div", { class: "persona-head" }, avatar(group.account),
      h("div", { class: "who" }, h("h3", {}, accountLabel(group)),
        h("div", { class: "sub" }, tn(group.profiles.length, "account.identityOne", "account.identityMany"))),
      login === null ? null : login ? badge("ok", t("account.signedIn")) : badge("warn", t("account.notSignedIn"))),
    group.profiles.length > 1 && authorDiffers(group) ? h("p", { class: "notice info" }, t("account.whySeveralLong")) : null,
    group.profiles.map((p) => identity(p, group, ctx)));
}

function identity(p, group, ctx) {
  const { state, run } = ctx;
  const box = h("section", { class: "identity", "aria-labelledby": `profile-${p.id}` });
  const pins = state.config.pins.filter((pin) => pin.profile === p.id);

  const folderForm = addPathForm({
    label: t("accounts.addFolder"), hint: t("accounts.addFolderHint", { id: p.id }),
    button: t("accounts.addFolderButton"),
    onAdd: async (typed) => {
      const dir = await normalizePath(typed);
      const owner = ownerOf(state, dir);
      if (owner && owner !== p.id && !(await confirm(t("accounts.moveAsk", { dir, id: p.id }),
        t("accounts.moveText", { dir, owner, id: p.id }), t("action.move")))) {
        return;
      }
      await run(() => api.addDir(dir, p.id), null); // the log says what changed
    },
  });
  const pinForm = addPathForm({
    label: t("accounts.pin"), hint: t("accounts.pinHint"), button: t("accounts.pinButton"),
    onAdd: async (typed) => {
      const repo = await normalizePath(typed);
      await run(() => api.addPin(repo, p.id), t("accounts.pinned", { repo, id: p.id }));
    },
  });

  const show = () => replace(box,
    h("div", { class: "identity-head" },
      h("div", { class: "who" },
        h("h4", { id: `profile-${p.id}` }, `${p.name} <${p.email}>`),
        h("span", { class: "chip" }, p.id),
        p.id === state.config.default ? badge("info", t("account.default")) : null),
      h("div", { class: "actions" },
        h("button", { type: "button", class: "small", onclick: edit, "aria-label": t("accounts.editLabel", { id: p.id }) }, t("action.edit")),
        h("button", { type: "button", class: "small danger", onclick: remove, "aria-label": t("accounts.removeLabel", { id: p.id }) },
          group.profiles.length > 1 ? t("accounts.removeIdentity") : t("accounts.removeProfile")))),
    p.ssh_key || p.signing?.method ? factList([
      [t("fact.sshKey"), p.ssh_key],
      [t("fact.signing"), p.signing?.method ? [t(SIGNING_LABELS[p.signing.method] ?? p.signing.method), p.signing.key].filter(Boolean).join(" · ") : ""],
    ]) : null,
    pathList({
      title: t("accounts.folders"), items: p.dirs, empty: pins.length > 0 ? t("account.onlyPins") : t("account.noDirs"), icon: "folder",
      removeLabel: (dir) => t("accounts.removeFolderLabel", { dir }),
      onRemove: async (dir) => await confirm(t("accounts.removeFolderAsk"),
        t("accounts.removeFolderText", { dir, id: p.id }), t("action.remove")) &&
        run(() => api.removeDir(dir), t("accounts.removedFolder", { dir })),
    }),
    pins.length > 0 ? pathList({
      title: t("accounts.pins"), items: pins.map((pin) => pin.repo), empty: "", icon: "pin",
      removeLabel: (repo) => t("accounts.unpinLabel", { repo }),
      onRemove: async (repo) => await confirm(t("accounts.unpinAsk"),
        t("accounts.unpinText", { repo }), t("action.unpin")) &&
        run(() => api.removePin(repo), t("accounts.unpinned", { repo })),
    }) : null,
    h("div", { class: "actions" }, revealer(t("accounts.addFolderButton"), folderForm), revealer(t("accounts.pin"), pinForm)),
    folderForm, pinForm);

  function edit() {
    replace(box, h("h4", { id: `profile-${p.id}` }, t("accounts.editTitle", { id: p.id })),
      profileForm({
        profile: p, accounts: state.accounts, onCancel: show,
        onSubmit: (changes) => run(() => api.updateProfile(p.id, changes), t("accounts.saved", { id: p.id })),
      }));
    box.querySelector("input")?.focus();
  }

  async function remove() {
    const parts = [p.dirs.length ? tn(p.dirs.length, "accounts.withFolderOne", "accounts.withFolderMany") : "",
      pins.length ? tn(pins.length, "accounts.withPinOne", "accounts.withPinMany") : ""].filter(Boolean);
    const text = [
      parts.length ? t("accounts.removeTextWith", { id: p.id, what: parts.join(t("list.and")) }) : t("accounts.removeText", { id: p.id }),
      p.id === state.config.default ? t("accounts.removeDefault", { id: p.id }) : "",
      t("accounts.undo"),
    ].filter(Boolean).join(" ");
    if (await confirm(t("accounts.removeAsk", { id: p.id }), text, t("accounts.removeProfile"))) {
      await run(() => api.removeProfile(p.id), t("accounts.removed", { id: p.id }));
    }
  }

  show();
  return box;
}

function defaultCard({ state, run }) {
  const select = h("select", { name: "default" },
    h("option", { value: "" }, t("default.none")),
    state.config.profiles.map((p) => h("option", { value: p.id, selected: p.id === state.config.default },
      `${p.id} · ${p.account} · ${p.name}`)));
  const save = h("button", { type: "submit" }, t("action.save"));
  const form = h("form", { class: "row default-form" },
    field(t("default.label"), select, t("default.hint")), save);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    busy(save, () => run(() => api.setDefault(select.value),
      select.value ? t("default.set", { id: select.value }) : t("default.removed")));
  });
  return h("div", { class: "card" }, h("h3", {}, t("default.title")), form);
}

function pathList({ title, items, empty, removeLabel, onRemove, icon: iconName }) {
  return [
    h("h4", {}, title),
    items.length === 0
      ? h("p", { class: "empty" }, empty)
      : h("ul", { class: "rows" }, items.map((item) => {
        const button = h("button", { type: "button", class: "small ghost danger-text", "aria-label": removeLabel(item) }, t("action.remove"));
        button.addEventListener("click", () => busy(button, () => onRemove(item)));
        return h("li", {}, h("span", { class: "mono path" }, icon(iconName), item), button);
      })),
  ];
}

// revealer is a button that shows a hidden form and moves focus into it.
function revealer(label, form) {
  const button = h("button", { type: "button", class: "small ghost add", "aria-expanded": "false" }, `+ ${label}`);
  button.addEventListener("click", () => {
    form.hidden = !form.hidden;
    button.setAttribute("aria-expanded", String(!form.hidden));
    if (!form.hidden) form.querySelector("input")?.focus();
  });
  return button;
}

function addPathForm({ label, hint, button, onAdd }) {
  const input = h("input", { type: "text", required: true, autocomplete: "off", spellcheck: "false" });
  const submit = h("button", { type: "submit", class: "primary" }, button);
  const form = h("form", { class: "row add-form", hidden: true }, field(label, input, hint), submit);
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
