// Repositories: every repository in the profiles' folders and pins, the
// author its commits use and the account each remote pushes as, compared
// with its profile, so a mix-up of accounts shows before a push.

import { groupByAccount, accountLabel } from "./accounts.js";
import { t, tn } from "../i18n.js";
import { api } from "../api.js";
import { h, replace, badge, field, busy } from "../dom.js";

const VERDICTS = {
  problem: { badge: "fail", label: "verdict.problem", order: 0 },
  unconfirmed: { badge: "warn", label: "verdict.unconfirmed", order: 1 },
  note: { badge: "info", label: "verdict.note", order: 2 },
  ok: { badge: "ok", label: "verdict.ok", order: 3 },
  unmanaged: { badge: "info", label: "verdict.unmanaged", order: 4 },
};

const FILTERS = [
  ["all", "filter.all"],
  ["problem", "filter.problem"],
  ["unconfirmed", "filter.unconfirmed"],
  ["note", "filter.note"],
  ["ok", "filter.ok"],
  ["unmanaged", "filter.unmanaged"],
];

// The last result survives a re-render of the page (after a change elsewhere).
let last = null;

export function renderRepositories(section, { state }) {
  const results = h("div", { class: "stack", "aria-live": "polite" });
  const dirs = h("input", { type: "text", name: "dirs", autocomplete: "off", spellcheck: "false",
    placeholder: "~/code, ~/Developer" });
  const ssh = h("input", { type: "checkbox", name: "ssh" });
  const submit = h("button", { type: "submit", class: "primary" }, t("repos.check"));
  const form = h("form", { class: "card stack" },
    h("p", {}, t("repos.intro")),
    field(t("repos.alsoCheck"), dirs, t("repos.alsoCheckHint")),
    h("label", { class: "check" }, ssh, t("repos.askSSH")),
    h("div", { class: "row" }, submit));

  const check = () => busy(submit, async () => {
    replace(results, h("p", { class: "muted" }, t("account.checking")));
    try {
      const folders = dirs.value.split(",").map((d) => d.trim()).filter(Boolean);
      const { data } = await api.verify(folders, ssh.checked);
      last = { report: data, filter: "all", query: "" };
      showReport(results, state);
    } catch (err) {
      replace(results, h("p", { class: "notice fail" }, err.message));
    }
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    check();
  });
  section.append(form, results);
  if (last) showReport(results, state);
  else check();
}

function showReport(results, state) {
  const { report } = last;
  const list = h("div", { class: "stack" });
  const search = h("input", { type: "search", name: "filter", autocomplete: "off", spellcheck: "false",
    placeholder: t("repos.searchPlaceholder"), value: last.query });
  const filters = h("div", { class: "filters", role: "group", "aria-label": t("repos.show") },
    FILTERS.map(([value, label]) => {
      const count = value === "all" ? report.repositories.length : report.repositories.filter((r) => r.verdict === value).length;
      const button = h("button", { type: "button", "aria-pressed": String(last.filter === value) }, `${t(label)} (${count})`);
      button.addEventListener("click", () => {
        last.filter = value;
        for (const b of filters.children) b.setAttribute("aria-pressed", String(b === button));
        drawList(list, state);
      });
      return button;
    }));
  search.addEventListener("input", () => {
    last.query = search.value;
    drawList(list, state);
  });
  replace(results, summary(report), h("div", { class: "card stack" }, filters, field(t("repos.filter"), search), list));
  drawList(list, state);
}

function summary(report) {
  const c = report.counts;
  const notices = [];
  if (report.missing.length) {
    notices.push(h("p", { class: "notice warn" }, t("repos.notFound", { dirs: report.missing.join(", ") })));
  }
  if (!report.complete) {
    notices.push(h("p", { class: "notice warn" }, t("repos.incomplete")));
  }
  const verdict = c.problems > 0
    ? h("p", { class: "notice fail" }, tn(c.problems, "alert.problemOne", "alert.problemMany"), t("repos.openBelow"))
    : h("p", { class: "notice ok" }, t("repos.noMixups"));
  return h("div", { class: "card stack" },
    h("div", { class: "row summary-badges" },
      badge("ok", tn(c.ok, "tally.okOne", "tally.okMany")), badge("fail", tn(c.problems, "tally.problemOne", "tally.problemMany")),
      badge("warn", tn(c.unconfirmed, "tally.unconfirmedOne", "tally.unconfirmedMany")), badge("info", tn(c.notes, "tally.noteOne", "tally.noteMany")),
      badge("info", tn(c.unmanaged, "repos.outsideOne", "repos.outsideMany"))),
    verdict,
    c.unconfirmed > 0 ? h("p", { class: "notice warn" }, tn(c.unconfirmed, "repos.unconfirmedOne", "repos.unconfirmedMany")) : null,
    h("p", { class: "muted" }, tn(report.repositories.length, "repos.inOne", "repos.inMany"), " ",
      h("span", { class: "mono" }, report.scanned.join(", "))),
    notices);
}

// drawList shows the repositories that pass the filter, grouped by profile,
// the most urgent first.
function drawList(list, state) {
  const query = last.query.trim().toLowerCase();
  const shown = last.report.repositories.filter((r) =>
    (last.filter === "all" || r.verdict === last.filter) && (!query || searchText(r).includes(query)));
  if (!shown.length) {
    replace(list, h("p", { class: "empty" }, t("repos.noMatch")));
    return;
  }
  // One group per account, as on the Accounts page; a repository outside
  // every profile is in its own group at the end.
  const accounts = groupByAccount(state.config ?? { profiles: [] });
  const groupOf = (r) => accounts.find((g) => g.profiles.some((p) => p.id === r.profile));
  const groups = new Map();
  for (const r of shown) {
    const g = groupOf(r);
    const key = g?.key ?? "";
    if (!groups.has(key)) groups.set(key, { group: g, repos: [] });
    groups.get(key).repos.push(r);
  }
  const order = [...groups.keys()].sort((a, b) => (a === "") - (b === "") || a.localeCompare(b));
  replace(list, order.map((key) => {
    const { group, repos } = groups.get(key);
    repos.sort((a, b) => VERDICTS[a.verdict].order - VERDICTS[b.verdict].order || a.path.localeCompare(b.path));
    return h("section", { class: "repo-group" },
      h("h3", {}, group ? accountLabel(group) : t("repos.outsideGroup"), h("span", { class: "muted" }, ` · ${repos.length}`)),
      repos.map((r) => repoDetails(r, group)));
  }));
}

function repoDetails(r, group) {
  const v = VERDICTS[r.verdict];
  const pushes = r.remotes.map((rc) => rc.push_as || "?");
  const details = h("details", { class: "change repo", open: r.verdict === "problem" || r.verdict === "unconfirmed" },
    h("summary", {},
      badge(v.badge, t(v.label)),
      h("span", { class: "mono" }, r.path),
      group && group.profiles.length > 1 ? h("span", { class: "muted" }, r.author) : null,
      pushes.length ? h("span", { class: "muted" }, t("repos.pushesAs", { accounts: [...new Set(pushes)].join(", ") })) : null),
    h("dl", { class: "facts" },
      r.profile ? [h("dt", {}, t("fact.profile")), h("dd", {}, `${r.profile} (${r.account})`)] : null,
      h("dt", {}, t("fact.why")), h("dd", {}, reasonText(r)),
      h("dt", {}, t("fact.author")), h("dd", {}, r.author || t("repos.noAuthor"), r.author_source ? h("span", { class: "muted" }, " ", t("repos.from", { file: r.author_source })) : null),
      r.profile_author && r.profile_author !== r.author ? [h("dt", {}, t("repos.profileAuthor")), h("dd", {}, r.profile_author)] : null),
    r.remotes.length ? remotesTable(r.remotes) : h("p", { class: "muted" }, t("repos.noRemote")),
    r.findings.length ? h("ul", { class: "checks" }, r.findings.map((f) =>
      h("li", {}, badge(FINDINGS[f.level]?.[0] ?? "info", t(FINDINGS[f.level]?.[1] ?? "verdict.note")), h("span", {}, f.text)))) : null);
  return details;
}

function remotesTable(remotes) {
  return h("div", { class: "table-wrap" },
    h("table", { class: "remotes" },
      h("thead", {}, h("tr", {}, ["repos.remote", "repos.address", "repos.pushAs", "repos.how"].map((key) => h("th", { scope: "col" }, t(key))))),
      h("tbody", {}, remotes.map((rc) => h("tr", {},
        h("td", {}, rc.name),
        h("td", { class: "mono" }, rc.address),
        h("td", {}, rc.push_as || t("repos.cannotTell"), rc.chosen_by ? h("div", { class: "muted" }, t("repos.chosenBy", { by: rc.chosen_by })) : null),
        h("td", {}, `${rc.protocol} · ${rc.via}`))))));
}

const FINDINGS = { problem: ["fail", "verdict.problem"], unconfirmed: ["warn", "verdict.unconfirmed"], note: ["info", "verdict.note"] };

function reasonText(r) {
  const text = t(`reason.${r.reason}`);
  return r.match ? `${text} ${r.match}` : text;
}

function searchText(r) {
  return [r.path, r.profile, r.account, r.author, ...r.remotes.flatMap((rc) => [rc.address, rc.push_as])]
    .filter(Boolean).join(" ").toLowerCase();
}
