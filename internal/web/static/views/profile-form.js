// Forms for adding a profile and changing one.

import { t } from "../i18n.js";
import { h, field, busy } from "../dom.js";

const SIGNING_METHODS = [
  ["", "form.signingUnset"],
  ["ssh", "form.signingSSH"],
  ["gpg", "form.signingGPG"],
  ["none", "form.signingOff"],
];

function textInput(name, value = "", extra = {}) {
  return h("input", { type: "text", name, value, autocomplete: "off", spellcheck: "false", ...extra });
}

// accountList offers the accounts signed in to gh as suggestions.
function accountList(id, accounts) {
  return h("datalist", { id }, accounts.map((a) => h("option", { value: a.login })));
}

// profileForm returns a form for a new profile (profile undefined) or for
// changes to an existing one. onSubmit receives only the changed fields.
export function profileForm({ profile, accounts, onSubmit, onCancel }) {
  const isNew = !profile;
  const p = profile ?? { id: "", account: "", name: "", email: "", ssh_key: "", signing: { method: "", key: "" } };
  const listId = `accounts-${p.id || "new"}`;
  const inputs = {
    id: textInput("id", p.id, { required: true, pattern: "[a-z0-9][a-z0-9\\-]{0,31}", maxlength: 32, title: t("form.idTitle") }),
    account: textInput("account", p.account, { required: true, list: listId, pattern: "[A-Za-z0-9][A-Za-z0-9._\\-]*", title: t("form.accountTitle") }),
    name: textInput("name", p.name, { required: true, autocomplete: "name" }),
    email: h("input", { type: "email", name: "email", value: p.email, required: true, autocomplete: "email" }),
    ssh_key: textInput("ssh_key", p.ssh_key),
    method: h("select", { name: "method" }, SIGNING_METHODS.map(([value, label]) =>
      h("option", { value, selected: (p.signing?.method ?? "") === value }, t(label)))),
    key: textInput("key", p.signing?.key ?? ""),
  };
  const submit = h("button", { type: "submit", class: "primary" }, isNew ? t("form.add") : t("form.save"));
  const form = h("form", { class: "stack" },
    isNew ? field(t("form.id"), inputs.id, t("form.idHint")) : null,
    field(t("form.account"), inputs.account, t("form.accountHint")),
    accountList(listId, accounts),
    field(t("form.name"), inputs.name),
    field(t("form.email"), inputs.email, t("form.emailHint")),
    isNew ? null : h("fieldset", {},
      h("legend", {}, t("form.optional")),
      h("div", { class: "stack" },
        field(t("form.sshKey"), inputs.ssh_key, t("form.sshKeyHint")),
        field(t("form.signing"), inputs.method),
        field(t("form.signingKey"), inputs.key, t("form.signingKeyHint")))),
    h("div", { class: "actions" }, submit,
      onCancel ? h("button", { type: "button", onclick: onCancel }, t("dialog.cancel")) : null));

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const values = Object.fromEntries(Object.entries(inputs).map(([k, el]) => [k, el.value.trim()]));
    busy(submit, () => onSubmit(isNew ? newProfile(values) : changes(p, values)));
  });
  return form;
}

function newProfile(v) {
  return { id: v.id, account: v.account, name: v.name, email: v.email };
}

// changes keeps only the fields that differ, so untouched keys stay as written.
function changes(p, v) {
  const out = {};
  for (const key of ["account", "name", "email", "ssh_key"]) {
    if (v[key] !== (p[key] ?? "")) out[key] = v[key];
  }
  const method = p.signing?.method ?? "";
  const key = p.signing?.key ?? "";
  if (v.method !== method || v.key !== key) {
    out.signing = { method: v.method, key: v.method === "" || v.method === "none" ? "" : v.key };
  }
  return out;
}
