// GitPersona configuration page: loads the state, routes between sections by
// the URL hash and re-renders after every change or language switch.

import { api, ApiError } from "./api.js";
import { h, replace, status, clearStatus } from "./dom.js";
import { t, lang, setLang, applyDocumentLang, LANGUAGES } from "./i18n.js";
import { icon } from "./icons.js";
import { renderOverview } from "./views/overview.js";
import { renderProfiles } from "./views/profiles.js";
import { renderApply } from "./views/apply.js";
import { renderShells } from "./views/shells.js";
import { renderBackups } from "./views/backups.js";
import { renderSetup } from "./views/setup.js";
import { renderRepositories } from "./views/repositories.js";

const VIEWS = {
  overview: { title: "nav.overview", icon: "home", render: renderOverview },
  profiles: { title: "nav.accounts", icon: "user", render: renderProfiles, needsConfig: true },
  repositories: { title: "nav.repositories", icon: "repo", render: renderRepositories, needsConfig: true },
  apply: { title: "nav.apply", icon: "diff", render: renderApply, needsConfig: true },
  shells: { title: "nav.shells", icon: "terminal", render: renderShells },
  backups: { title: "nav.backups", icon: "clock", render: renderBackups },
  setup: { title: "nav.setup", icon: "wand", render: renderSetup, hidden: true },
};
const DEFAULT_VIEW = "overview";
// NO_CHANGE matches output that says a change was already in place; it
// counts only when that is all the command printed. Both languages.
const NO_CHANGE = /^[^\n]*(nothing to change|already up to date|无需改动|已是最新)[^\n]*$/i;

let state = null;
let closed = false; // after Close the page shows only the stopped message

function currentView() {
  const name = location.hash.slice(1);
  if (Object.hasOwn(VIEWS, name)) return name;
  // Without a configuration, the page opens on the setup steps.
  return state && !state.config_exists ? "setup" : DEFAULT_VIEW;
}

// loadState reads the page's state. Opening or reloading the page asks gh
// about the accounts again (fresh); after a change the server reuses its
// recent answer, so a run of changes does not query the host each time.
async function loadState(fresh = false) {
  const { data } = await api.state(fresh);
  state = data;
}

// run performs a change, reports the result in the live region and shows
// the refreshed state. It returns whether the change succeeded.
async function run(action, success) {
  try {
    const { log } = await action();
    await loadState();
    render();
    // Without a message of its own, the command's output is the message.
    if (NO_CHANGE.test(log.trim())) status("ok", t("status.nothingToChange"), log);
    else if (success) status("ok", success, log);
    else status("ok", log.trim() || t("status.done"));
    focusResult();
    return true;
  } catch (err) {
    const changed = await changedElsewhere();
    status("fail", changed ? `${err.message}\n${t("status.changedElsewhere")}`
      : err.message, err instanceof ApiError ? err.log : "");
    focusResult();
    return false;
  }
}

// changedElsewhere reloads the state after a failed change and redraws the
// page when the configuration was edited outside it, so it never keeps
// showing profiles that are gone. An unchanged page keeps what was typed.
async function changedElsewhere() {
  const before = JSON.stringify(state?.config);
  try {
    await loadState();
  } catch {
    return false;
  }
  if (JSON.stringify(state.config) === before) return false;
  render();
  return true;
}

// chrome draws the parts around the content in the page's language.
function chrome(name) {
  document.getElementById("skip-link").textContent = t("app.skip");
  document.getElementById("tagline").textContent = t("app.tagline");
  const close = document.getElementById("close");
  if (close) close.textContent = t("app.close");
  const switcher = document.getElementById("lang-switch");
  switcher.setAttribute("aria-label", t("app.language"));
  replace(switcher, LANGUAGES.map(([code, label]) => h("button", {
    type: "button", "aria-pressed": String(code === lang()), lang: code === "zh" ? "zh-CN" : "en",
    onclick: () => switchLang(code),
  }, label)));
  const nav = document.getElementById("nav");
  if (!nav || closed) return;
  nav.setAttribute("aria-label", t("app.sections"));
  replace(nav, h("ul", {}, Object.entries(VIEWS).filter(([, v]) => !v.hidden || name === "setup").map(([key, v]) =>
    h("li", {}, h("a", { href: `#${key}`, "aria-current": key === name ? "page" : null },
      icon(v.icon), h("span", {}, t(v.title)),
      key === "overview" && state && !state.doctor.ok ? h("span", { class: "count", "aria-label": t("nav.needsAttention") }, "!") : null)))));
}

async function switchLang(code) {
  if (code === lang()) return;
  setLang(code);
  clearStatus();
  // Messages from GitPersona come in the page's language: ask again.
  try {
    await loadState();
  } catch {
    // The page shows the last state in the new language.
  }
  render();
}

function render({ focus = false } = {}) {
  const name = currentView();
  const view = VIEWS[name];
  document.title = `${t(view.title)} · GitPersona`;
  chrome(name);
  const heading = h("h2", { tabindex: "-1" }, t(view.title));
  const section = h("section", { "aria-labelledby": "view-title" }, heading);
  heading.id = "view-title";
  if (view.needsConfig && !state.config_exists) {
    section.append(h("p", { class: "empty" }, t("app.noConfig"), " ",
      h("a", { href: "#setup" }, t("app.setUp")), t("app.setUpAfter")));
  } else {
    view.render(section, { state, run, refresh: refreshAll });
  }
  replace(document.getElementById("view"), section);
  // A link to a part of a section (such as #overview-health) scrolls to it.
  const part = location.hash.slice(1);
  if (part && !Object.hasOwn(VIEWS, part)) document.getElementById(part)?.scrollIntoView();
  else if (focus) heading.focus();
}

// focusResult moves keyboard focus to the result message when the control
// that started the action is gone (the view was drawn again).
function focusResult() {
  if (document.activeElement && document.activeElement !== document.body) return;
  document.querySelector("#status .notice")?.focus();
}

async function refreshAll() {
  try {
    await loadState();
    render();
  } catch (err) {
    status("fail", err.message);
  }
}

async function closePage() {
  try {
    await api.close();
  } catch {
    // The server may already be gone; the page is closed either way.
  }
  closed = true;
  document.getElementById("nav")?.remove();
  document.getElementById("close")?.remove();
  replace(document.getElementById("view"),
    h("section", {}, h("h2", {}, t("app.stopped")), h("p", {}, t("app.stoppedHint"))));
  clearStatus();
}

async function start() {
  applyDocumentLang();
  chrome(currentView());
  document.getElementById("close").addEventListener("click", closePage);
  window.addEventListener("hashchange", () => {
    if (closed) return;
    clearStatus();
    render({ focus: true });
  });
  try {
    await loadState(true);
    render();
  } catch (err) {
    replace(document.getElementById("view"), h("div", { class: "notice fail", role: "alert" }, err.message));
  }
}

start();
