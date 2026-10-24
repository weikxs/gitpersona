// Repositories: every repository in the profiles' folders and pins, the
// author its commits use and the account each remote pushes as, compared
// with its profile, so a mix-up of accounts shows before a push.

import { api } from "../api.js";
import { h, replace, badge, field, busy } from "../dom.js";

const VERDICTS = {
  problem: { badge: "fail", label: "Problem", order: 0 },
  unconfirmed: { badge: "warn", label: "Unconfirmed", order: 1 },
  note: { badge: "info", label: "Note", order: 2 },
  ok: { badge: "ok", label: "As expected", order: 3 },
  unmanaged: { badge: "info", label: "Outside profiles", order: 4 },
};

const FILTERS = [
  ["all", "All"],
  ["problem", "Problems"],
  ["unconfirmed", "Unconfirmed"],
  ["note", "Notes"],
  ["ok", "As expected"],
  ["unmanaged", "Outside profiles"],
];

// The last result survives a re-render of the page (after a change elsewhere).
let last = null;

export function renderRepositories(section, { state }) {
  const results = h("div", { class: "stack", "aria-live": "polite" });
  const dirs = h("input", { type: "text", name: "dirs", autocomplete: "off", spellcheck: "false",
    placeholder: "~/code, ~/Developer" });
  const ssh = h("input", { type: "checkbox", name: "ssh" });
  const submit = h("button", { type: "submit", class: "primary" }, "Check repositories");
  const form = h("form", { class: "card stack" },
    h("p", {}, "For every repository in your profiles' folders and pins: the author its commits use and the account " +
      "each remote pushes as, next to the profile that should decide. GitPersona reads only Git and SSH settings: " +
      "it fetches no token, opens no key and connects nowhere."),
    field("Also check these folders", dirs, "Comma-separated, for repositories outside your profiles; absolute or starting with ~/"),
    h("label", { class: "check" }, ssh,
      "Ask the hosts over SSH which account each key signs in as. ssh connects once per key and host, as a push " +
      "would; GitPersona never reads the key."),
    h("div", { class: "row" }, submit));

  const check = () => busy(submit, async () => {
    replace(results, h("p", { class: "muted" }, "Checking repositories…"));
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
    placeholder: "Path, account or email", value: last.query });
  const filters = h("div", { class: "filters", role: "group", "aria-label": "Show" },
    FILTERS.map(([value, label]) => {
      const count = value === "all" ? report.repositories.length : report.repositories.filter((r) => r.verdict === value).length;
      const button = h("button", { type: "button", "aria-pressed": String(last.filter === value) }, `${label} (${count})`);
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
  replace(results, summary(report), h("div", { class: "card stack" }, filters, field("Filter", search), list));
  drawList(list, state);
}

function summary(report) {
  const c = report.counts;
  const notices = [];
  if (report.missing.length) {
    notices.push(h("p", { class: "notice warn" }, "Not found: ", report.missing.join(", ")));
  }
  if (!report.complete) {
    notices.push(h("p", { class: "notice warn" }, "The scan stopped early or did not go deep enough; some repositories may be missing."));
  }
  const verdict = c.problems > 0
    ? h("p", { class: "notice fail" }, `${c.problems} ${plural(c.problems, "repository", "repositories")} would commit or push as another identity. Open them below.`)
    : h("p", { class: "notice ok" }, "No repository commits or pushes as another identity.");
  return h("div", { class: "card stack" },
    h("div", { class: "row summary-badges" },
      badge("ok", `${c.ok} as expected`), badge("fail", `${c.problems} with problems`),
      badge("warn", `${c.unconfirmed} unconfirmed`), badge("info", `${c.notes} with notes`),
      badge("info", `${c.unmanaged} outside profiles`)),
    verdict,
    c.unconfirmed > 0 ? h("p", { class: "notice warn" }, `${c.unconfirmed} ${plural(c.unconfirmed, "repository pushes", "repositories push")} ` +
      "over SSH with a key no profile names, so the account cannot be read from your settings. Tick \"Ask the hosts over SSH\" " +
      "and check again to confirm it.") : null,
    h("p", { class: "muted" }, `${report.repositories.length} ${plural(report.repositories.length, "repository", "repositories")} in `,
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
    replace(list, h("p", { class: "empty" }, "No repository matches."));
    return;
  }
  const groups = new Map();
  for (const r of shown) {
    const key = r.profile || "";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(r);
  }
  const profiles = state.config?.profiles ?? [];
  const order = [...groups.keys()].sort((a, b) => (a === "") - (b === "") || a.localeCompare(b));
  replace(list, order.map((key) => {
    const repos = groups.get(key).sort((a, b) => VERDICTS[a.verdict].order - VERDICTS[b.verdict].order || a.path.localeCompare(b.path));
    return h("section", { class: "repo-group" },
      h("h3", {}, groupTitle(key, profiles), h("span", { class: "muted" }, ` · ${repos.length}`)),
      repos.map(repoDetails));
  }));
}

function groupTitle(id, profiles) {
  if (!id) return "Outside your profiles (Git uses your own settings)";
  const p = profiles.find((x) => x.id === id);
  return p ? `${id} · ${p.account} · ${p.name} <${p.email}>` : id;
}

function repoDetails(r) {
  const v = VERDICTS[r.verdict];
  const pushes = r.remotes.map((rc) => rc.push_as || "?");
  const details = h("details", { class: "change repo", open: r.verdict === "problem" || r.verdict === "unconfirmed" },
    h("summary", {},
      badge(v.badge, v.label),
      h("span", { class: "mono" }, r.path),
      pushes.length ? h("span", { class: "muted" }, `pushes as ${[...new Set(pushes)].join(", ")}`) : null),
    h("dl", { class: "facts" },
      r.profile ? [h("dt", {}, "Profile"), h("dd", {}, `${r.profile} (${r.account})`)] : null,
      h("dt", {}, "Why"), h("dd", {}, reasonText(r)),
      h("dt", {}, "Author"), h("dd", {}, r.author || "none", r.author_source ? h("span", { class: "muted" }, ` from ${r.author_source}`) : null),
      r.profile_author && r.profile_author !== r.author ? [h("dt", {}, "Profile author"), h("dd", {}, r.profile_author)] : null),
    r.remotes.length ? remotesTable(r.remotes) : h("p", { class: "muted" }, "No remote."),
    r.findings.length ? h("ul", { class: "checks" }, r.findings.map((f) =>
      h("li", {}, badge(FINDINGS[f.level]?.[0] ?? "info", FINDINGS[f.level]?.[1] ?? "Note"), h("span", {}, f.text)))) : null);
  return details;
}

function remotesTable(remotes) {
  return h("div", { class: "table-wrap" },
    h("table", { class: "remotes" },
      h("thead", {}, h("tr", {}, ["Remote", "Address", "Pushes as", "How"].map((t) => h("th", { scope: "col" }, t)))),
      h("tbody", {}, remotes.map((rc) => h("tr", {},
        h("td", {}, rc.name),
        h("td", { class: "mono" }, rc.address),
        h("td", {}, rc.push_as || "cannot tell", rc.chosen_by ? h("div", { class: "muted" }, `chosen by ${rc.chosen_by}`) : null),
        h("td", {}, `${rc.protocol} · ${rc.via}`))))));
}

const FINDINGS = { problem: ["fail", "Problem"], unconfirmed: ["warn", "Unconfirmed"], note: ["info", "Note"] };

const REASONS = { pin: "pinned repository", rule: "directory rule", default: "default profile", none: "no rule matches" };

function reasonText(r) {
  const text = REASONS[r.reason] ?? r.reason;
  return r.match ? `${text} ${r.match}` : text;
}

function searchText(r) {
  return [r.path, r.profile, r.account, r.author, ...r.remotes.flatMap((rc) => [rc.address, rc.push_as])]
    .filter(Boolean).join(" ").toLowerCase();
}

function plural(n, one, many) {
  return n === 1 ? one : many;
}
