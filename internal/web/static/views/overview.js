// Overview: first-run setup, which identity applies to a directory, the
// signed-in accounts and the health checks.

import { api } from "../api.js";
import { h, replace, badge, field, busy } from "../dom.js";

export function renderOverview(section, { state }) {
  if (!state.config_exists) {
    section.append(setupCard());
  } else if (state.config_error) {
    section.append(h("div", { class: "notice fail", role: "alert" },
      h("strong", {}, "The configuration has a problem. "), state.config_error));
  }
  section.append(resolveCard(), accountsCard(state), doctorCard(state.doctor));
}

function setupCard() {
  return h("div", { class: "card" },
    h("h3", {}, "Get started"),
    h("p", {}, "No configuration yet. Setup takes three steps: confirm one profile for each account signed in to gh, " +
      "choose the directories each one covers, then review and apply."),
    h("a", { class: "button primary", href: "#setup" }, "Start setup"));
}

function resolveCard() {
  const input = h("input", { type: "text", name: "dir", required: true, autocomplete: "off",
    spellcheck: "false", placeholder: "~/code/project" });
  const result = h("div", { "aria-live": "polite" });
  const submit = h("button", { type: "submit" }, "Check");
  const form = h("form", { class: "row" },
    field("Directory", input, "An absolute path or one starting with ~/"), submit);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    busy(submit, async () => {
      try {
        const { data } = await api.resolve(input.value.trim());
        replace(result, resolution(data));
      } catch (err) {
        replace(result, h("p", { class: "notice fail" }, err.message));
      }
    });
  });
  return h("div", { class: "card" },
    h("h3", {}, "Which identity applies here?"),
    form, result);
}

const REASONS = { pin: "pinned repository", rule: "directory rule", default: "default profile", none: "no rule matches" };

function reasonText(reason, match) {
  const text = REASONS[reason] ?? reason;
  return match ? `${text} (${match})` : text;
}

function resolution(r) {
  if (!r.found) {
    return h("p", { class: "notice warn" }, badge("warn", "No profile"), " ", reasonText(r.reason));
  }
  const facts = [
    ["Profile", r.profile],
    ["Account", r.account],
    ["Author", `${r.name} <${r.email}>`],
    ["Why", reasonText(r.reason, r.match)],
  ];
  if (r.new_repositories) {
    facts.push(["New repositories", `${r.new_repositories.profile} (${reasonText(r.new_repositories.reason, r.new_repositories.match)})`]);
  }
  const warnings = [];
  if (r.env_override) warnings.push(`${r.env_override} is set and overrides the author email.`);
  if (r.local_email) warnings.push(`This repository sets user.email = ${r.local_email} locally, which overrides the profile.`);
  if (r.in_repository && !r.consistent && !r.env_override && !r.local_email) {
    warnings.push(`Git currently resolves the email to ${r.effective_email || "nothing"}; run apply to update Git's configuration.`);
  }
  return h("div", {},
    factList(facts),
    warnings.map((w) => h("p", { class: "notice warn" }, badge("warn"), " ", w)));
}

export function factList(pairs) {
  return h("dl", { class: "facts" },
    pairs.filter(([, value]) => value).map(([term, value]) => [h("dt", {}, term), h("dd", {}, value)]));
}

function accountsCard(state) {
  const body = [];
  if (state.accounts_error) {
    body.push(h("p", { class: "notice warn" }, badge("warn"), " ", state.accounts_error));
  }
  if (state.accounts.length === 0 && !state.accounts_error) {
    body.push(h("p", { class: "empty" }, "No account is signed in to gh. Run `gh auth login` for each account."));
  }
  if (state.accounts.length > 0) {
    body.push(h("ul", { class: "rows" }, state.accounts.map((a) =>
      h("li", {}, h("span", { class: "mono" }, a.login),
        a.state === "success" ? badge("ok", "Signed in") : badge("fail", a.state || "Signed out")))));
  }
  return h("div", { class: "card" },
    h("h3", {}, "GitHub accounts in gh"),
    h("p", { class: "muted" }, "Read with a status query; GitPersona never reads or stores tokens here."),
    body);
}

// adviceLink points advice that names a command to the part of the page
// that does the same.
function adviceLink(advice) {
  if (advice.includes("install-shell")) return [" ", h("a", { href: "#shells" }, "Set up shells here")];
  if (advice.includes("gitpersona apply")) return [" ", h("a", { href: "#apply" }, "Review and apply here")];
  return null;
}

export function doctorCard(doctor) {
  const warnings = doctor.checks.some((c) => c.status === "warn");
  const summary = !doctor.ok ? badge("fail", "Needs attention")
    : warnings ? badge("warn", "Works, with warnings") : badge("ok", "All checks pass");
  return h("div", { class: "card" },
    h("div", { class: "card-head" }, h("h3", {}, "Health checks"), summary),
    h("ul", { class: "checks" }, doctor.checks.map((c) =>
      h("li", {}, badge(c.status), h("span", {}, c.label), c.advice ? h("span", { class: "advice" }, c.advice, adviceLink(c.advice)) : null))),
    doctor.ok ? null : h("p", { class: "muted" }, "Run `gitpersona doctor --fix` in a terminal to repair what can be fixed automatically."));
}

