// Overview: a dashboard. Numbers first (accounts, repositories, identity
// mix-ups, health), then what needs attention, then one card per account
// with the folders it covers, then the identity lookup and health checks.

import { api } from "../api.js";
import { h, replace, badge, field, busy, confirm } from "../dom.js";
import { t, tn } from "../i18n.js";
import { icon } from "../icons.js";
import { groupByAccount, accountLabel, signedIn, avatar, authorDiffers } from "./accounts.js";

// The last repository check, kept while the page is open (it takes a few
// seconds); a change on the page asks again.
let verified = null;
let verifying = null;

export function renderOverview(section, { state, run }) {
  if (!state.config_exists) {
    section.append(setupCard());
    section.append(accountsCard(state), doctorCard(state.doctor, run));
    return;
  }
  if (state.config_error) section.append(configProblem(state));
  section.append(h("p", { class: "lead" }, t("overview.lead")));
  const stats = h("div", { class: "stats" });
  const alerts = h("div", {});
  const accounts = h("div", {});
  section.append(stats, alerts,
    h("div", { class: "section-title" }, h("h3", {}, t("overview.accounts")),
      h("a", { href: "#profiles" }, t("overview.manageAccounts"))),
    accounts,
    h("div", { class: "section-title" }, h("h3", {}, t("overview.tools"))),
    h("div", { class: "grid" }, resolveCard(), doctorCard(state.doctor, run)));
  const draw = () => {
    replace(stats, statTiles(state, verified));
    replace(alerts, attention(state, verified));
    replace(accounts, h("div", { class: "grid" }, groupByAccount(state.config).map((g) => accountCard(g, state, verified))));
  };
  draw();
  if (!verified) {
    verifying ??= api.verify([], false).then(({ data }) => data).finally(() => { verifying = null; });
    verifying.then((data) => {
      verified = data;
      if (stats.isConnected) draw();
    }).catch(() => {
      if (stats.isConnected) replace(alerts, h("p", { class: "notice warn" }, t("overview.verifyFailed")), attention(state, null));
    });
  }
}

// forgetVerification makes the dashboard check the repositories again.
export function forgetVerification() {
  verified = null;
}

function statTiles(state, report) {
  const groups = groupByAccount(state.config);
  const pending = !report;
  const problems = report?.counts.problems ?? 0;
  const notes = report ? report.counts.notes + report.counts.unconfirmed : 0;
  const health = !state.doctor.ok ? "fail" : state.doctor.checks.some((c) => c.status === "warn") ? "warn" : "ok";
  return [
    stat("#profiles", groups.length, t("stat.accounts"), ""),
    stat("#repositories", pending ? "…" : report.repositories.length, t("stat.repositories"), pending ? "pending" : ""),
    stat("#repositories", pending ? "…" : problems, t("stat.mixups"), pending ? "pending" : problems > 0 ? "fail" : "ok"),
    stat("#repositories", pending ? "…" : notes, t("stat.notes"), pending ? "pending" : notes > 0 ? "warn" : "ok"),
    stat("#overview-health", t(`health.${health}`), t("stat.health"), health),
  ];
}

function stat(href, value, label, kind) {
  const word = typeof value === "string" && value !== "…";
  return h("a", { class: `stat ${kind}`, href },
    h("span", { class: word ? "value word" : "value" }, String(value)), h("span", { class: "label" }, label));
}

// attention lists what needs the user first: failing health checks and
// repositories that would commit or push as someone else.
function attention(state, report) {
  const failing = state.doctor.checks.filter((c) => c.status === "fail");
  const problems = report?.repositories.filter((r) => r.verdict === "problem") ?? [];
  const out = [];
  if (problems.length > 0) {
    out.push(alert("fail", tn(problems.length, "alert.problemOne", "alert.problemMany"),
      problems.slice(0, 5).map((r) => h("li", {}, h("span", { class: "mono" }, r.path), " · ", r.findings.find((f) => f.level === "problem")?.text ?? "")),
      h("a", { class: "button small", href: "#repositories" }, t("alert.open"))));
  }
  if (failing.length > 0) {
    out.push(alert("fail", tn(failing.length, "alert.checkOne", "alert.checkMany"),
      failing.map((c) => h("li", {}, c.label)),
      h("a", { class: "button small", href: "#overview-health" }, t("alert.open"))));
  }
  if (report && out.length === 0) {
    out.push(alert("ok", t("alert.allGood", { n: report.repositories.length }), null, null));
  }
  return out;
}

function alert(kind, title, items, action) {
  return h("div", { class: `alert ${kind}`, role: kind === "fail" ? "alert" : null },
    icon(kind === "ok" ? "shield" : "alert"),
    h("div", {}, h("p", { class: "title" }, title), items?.length ? h("ul", {}, items) : null),
    action);
}

// accountCard shows one account: its identities, the folders and pins
// each covers, and how its repositories check out.
function accountCard(group, state, report) {
  const ids = new Set(group.profiles.map((p) => p.id));
  const repos = report?.repositories.filter((r) => ids.has(r.profile)) ?? [];
  const pins = state.config.pins.filter((pin) => ids.has(pin.profile));
  const login = signedIn(group, state);
  return h("article", { class: "card persona", "aria-label": accountLabel(group) },
    h("div", { class: "persona-head" }, avatar(group.account),
      h("div", { class: "who" }, h("h4", {}, accountLabel(group)),
        h("div", { class: "sub" }, group.profiles.length > 1
          ? tn(group.profiles.length, "account.identityOne", "account.identityMany")
          : `${group.profiles[0].name} <${group.profiles[0].email}>`)),
      login === null ? null : login ? badge("ok", t("account.signedIn")) : badge("warn", t("account.notSignedIn"))),
    group.profiles.length > 1 && authorDiffers(group) ? h("p", { class: "muted" }, t("account.whySeveral")) : null,
    group.profiles.map((p) => identityBlock(p, group, state)),
    pins.length > 0 ? h("p", { class: "muted" }, icon("pin"), " ", tn(pins.length, "account.pinOne", "account.pinMany")) : null,
    h("div", { class: "foot" }, tally(repos, report), state.config.default && ids.has(state.config.default) ? badge("info", t("account.default")) : null));
}

function identityBlock(p, group, state) {
  return h("div", {},
    group.profiles.length > 1 ? h("p", { class: "muted" }, h("strong", {}, `${p.name} <${p.email}>`), ` · ${p.id}`) : null,
    p.dirs.length === 0
      ? h("p", { class: "empty" }, state.config.pins.some((pin) => pin.profile === p.id) ? t("account.onlyPins") : t("account.noDirs"))
      : h("ul", { class: "dirs" }, p.dirs.map((d) => h("li", {}, icon("folder"), d))));
}

// tally shows how the account's repositories check out.
function tally(repos, report) {
  if (!report) return h("span", { class: "muted" }, t("account.checking"));
  if (repos.length === 0) return h("span", { class: "muted" }, t("account.noRepos"));
  const count = (v) => repos.filter((r) => r.verdict === v).length;
  const ok = count("ok");
  const notes = count("note");
  const unconfirmed = count("unconfirmed");
  const problems = count("problem");
  const part = (n, kind) => (n > 0 ? h("span", { class: kind, title: String(n) }) : null);
  const meter = h("div", { class: "meter", "aria-hidden": "true" }, [
    part(ok, "ok"), part(notes, "note"), part(unconfirmed, "warn"), part(problems, "fail")]);
  // Widths follow the counts; set through the CSS object model (the policy
  // forbids style attributes, not this).
  [ok, notes, unconfirmed, problems].filter((n) => n > 0).forEach((n, i) => {
    meter.children[i].style.width = `${(100 * n) / repos.length}%`;
  });
  return h("div", { class: "stack" },
    h("div", { class: "tally" },
      badge("ok", tn(ok, "tally.okOne", "tally.okMany")),
      notes ? badge("info", tn(notes, "tally.noteOne", "tally.noteMany")) : null,
      unconfirmed ? badge("warn", tn(unconfirmed, "tally.unconfirmedOne", "tally.unconfirmedMany")) : null,
      problems ? badge("fail", tn(problems, "tally.problemOne", "tally.problemMany")) : null),
    meter);
}

// configProblem explains a configuration that cannot be used and how to get
// back to a working one.
export function configProblem(state) {
  return h("div", { class: "notice fail", role: "alert" },
    h("strong", {}, t("config.problem")), " ", state.config_error,
    h("p", {}, t("config.fixBefore"), h("span", { class: "mono" }, state.config_path), t("config.fixMiddle"),
      h("a", { href: "#backups" }, t("config.restore")), t("config.fixAfter")));
}

function setupCard() {
  return h("div", { class: "card" },
    h("h3", {}, t("setup.getStarted")),
    h("p", {}, t("setup.getStartedText")),
    h("a", { class: "button primary", href: "#setup" }, t("setup.start")));
}

function resolveCard() {
  const input = h("input", { type: "text", name: "dir", required: true, autocomplete: "off",
    spellcheck: "false", placeholder: "~/code/project" });
  const result = h("div", { "aria-live": "polite" });
  const submit = h("button", { type: "submit" }, t("resolve.check"));
  const form = h("form", { class: "row" },
    field(t("resolve.directory"), input, t("resolve.hint")), submit);
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
    h("h3", {}, t("resolve.title")),
    form, result);
}

function reasonText(reason, match) {
  const text = t(`reason.${reason}`);
  return match ? `${text} (${match})` : text;
}

function resolution(r) {
  if (!r.found) {
    return h("p", { class: "notice warn" }, badge("warn", t("resolve.noProfile")), " ", reasonText(r.reason));
  }
  const facts = [
    [t("fact.profile"), r.profile],
    [t("fact.account"), r.account],
    [t("fact.author"), `${r.name} <${r.email}>`],
    [t("fact.why"), reasonText(r.reason, r.match)],
  ];
  if (r.new_repositories) {
    facts.push([t("fact.newRepositories"), `${r.new_repositories.profile} (${reasonText(r.new_repositories.reason, r.new_repositories.match)})`]);
  }
  const warnings = [];
  if (r.env_override) warnings.push(t("resolve.envOverride", { name: r.env_override }));
  if (r.local_email) warnings.push(t("resolve.localEmail", { email: r.local_email }));
  if (r.in_repository && !r.consistent && !r.env_override && !r.local_email) {
    warnings.push(t("resolve.effective", { email: r.effective_email || t("resolve.nothing") }));
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
    body.push(h("p", { class: "empty" }, t("gh.none")));
  }
  if (state.accounts.length > 0) {
    body.push(h("ul", { class: "rows" }, state.accounts.map((a) =>
      h("li", {}, h("span", { class: "mono" }, a.login),
        a.state === "success" ? badge("ok", t("account.signedIn")) : badge("fail", a.state || t("gh.signedOut"))))));
  }
  return h("div", { class: "card" },
    h("h3", {}, t("gh.title")),
    h("p", { class: "muted" }, t("gh.note")),
    body);
}

// adviceLink points advice that names a command to the part of the page
// that does the same. Commands are never translated, so this works in both
// languages.
function adviceLink(advice) {
  if (advice.includes("install-shell")) return [" ", h("a", { href: "#shells" }, t("advice.shells"))];
  if (advice.includes("gitpersona apply")) return [" ", h("a", { href: "#apply" }, t("advice.apply"))];
  return null;
}

// FIXES are the repairs the page runs from a health check.
const FIXES = {
  "old-rules": {
    label: "fix.oldRules", run: () => api.removeOldRules(),
    ask: ["fix.oldRulesAsk", "fix.oldRulesAskText", "fix.remove"],
    done: "fix.oldRulesDone",
  },
};

export function doctorCard(doctor, run) {
  const warnings = doctor.checks.some((c) => c.status === "warn");
  const summary = !doctor.ok ? badge("fail", t("health.fail"))
    : warnings ? badge("warn", t("health.warn")) : badge("ok", t("health.ok"));
  return h("div", { class: "card", id: "overview-health" },
    h("div", { class: "card-head" }, h("h3", {}, t("health.title")), summary),
    h("ul", { class: "checks" }, doctor.checks.map((c) =>
      h("li", {}, badge(c.status), h("span", {}, c.label), c.advice ? h("span", { class: "advice" }, c.advice, adviceLink(c.advice)) : null,
        fixButton(c, run)))),
    doctor.ok ? null : h("p", { class: "muted" }, t("health.fixHint")));
}

function fixButton(check, run) {
  const fix = FIXES[check.fix];
  if (!fix || !run) return null;
  const button = h("button", { type: "button" }, t(fix.label));
  button.addEventListener("click", () => busy(button, async () => {
    if (await confirm(...fix.ask.map((key) => t(key)))) {
      forgetVerification();
      await run(fix.run, t(fix.done));
    }
  }));
  return h("div", {}, button);
}
