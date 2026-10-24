// Accounts: profiles grouped by the account they push as. One account can
// have several profiles when folders commit under different author names or
// emails; the page shows them under that one account.

import { h } from "../dom.js";
import { t } from "../i18n.js";

// groupByAccount returns one entry per account (host and login), in the
// order the profiles are written, each with its profiles.
export function groupByAccount(config) {
  const groups = [];
  for (const p of config.profiles) {
    const host = p.host || "github.com";
    const key = `${host}/${p.account.toLowerCase()}`;
    let group = groups.find((g) => g.key === key);
    if (!group) {
      group = { key, account: p.account, host, provider: p.provider || "github", profiles: [] };
      groups.push(group);
    }
    group.profiles.push(p);
  }
  return groups;
}

// accountLabel names an account, with its host when that is not GitHub.
export function accountLabel(group) {
  return group.host === "github.com" ? group.account : t("account.onHost", { account: group.account, host: group.host });
}

// signedIn tells a GitHub account's gh status: true, false, or null when
// gh does not handle this host.
export function signedIn(group, state) {
  if (group.host !== "github.com") return null;
  return state.accounts.some((a) => a.login.toLowerCase() === group.account.toLowerCase() && a.state === "success");
}

// avatar shows the account's first letter on one of a few colors, chosen
// from the name so an account keeps its color.
export function avatar(name) {
  let sum = 0;
  for (const ch of name) sum = (sum + ch.codePointAt(0)) % 997;
  return h("span", { class: `avatar c${sum % 5}`, "aria-hidden": "true" }, [...name][0] ?? "?");
}

// authorDiffers reports whether the profiles of one account commit under
// different names or emails, which is why it has more than one profile.
export function authorDiffers(group) {
  return new Set(group.profiles.map((p) => `${p.name}\u0000${p.email}`)).size > 1;
}
