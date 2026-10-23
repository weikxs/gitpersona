// GitPersona configuration page: loads the state, routes between sections by
// the URL hash and re-renders after every change.

import { api, ApiError } from "./api.js";
import { h, replace, status, clearStatus } from "./dom.js";
import { renderOverview } from "./views/overview.js";
import { renderProfiles } from "./views/profiles.js";
import { renderApply } from "./views/apply.js";
import { renderShells } from "./views/shells.js";
import { renderBackups } from "./views/backups.js";
import { renderSetup } from "./views/setup.js";

const VIEWS = {
  overview: { title: "Overview", render: renderOverview },
  profiles: { title: "Profiles", render: renderProfiles, needsConfig: true },
  apply: { title: "Review & apply", render: renderApply, needsConfig: true },
  shells: { title: "Shells", render: renderShells },
  backups: { title: "Backups", render: renderBackups },
  setup: { title: "Set up GitPersona", render: renderSetup },
};
const DEFAULT_VIEW = "overview";
// NO_CHANGE matches output that says a change was already in place; it
// counts only when that is all the command printed.
const NO_CHANGE = /^[^\n]*(nothing to change|already up to date)[^\n]*$/i;

let state = null;
let closed = false; // after Close the page shows only the stopped message

function currentView() {
  const name = location.hash.slice(1);
  if (Object.hasOwn(VIEWS, name)) return name;
  // Without a configuration, the page opens on the setup steps.
  return state && !state.config_exists ? "setup" : DEFAULT_VIEW;
}

async function loadState() {
  const { data } = await api.state();
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
    if (NO_CHANGE.test(log.trim())) status("ok", "Nothing to change.", log);
    else if (success) status("ok", success, log);
    else status("ok", log.trim() || "Done.");
    focusResult();
    return true;
  } catch (err) {
    const changed = await changedElsewhere();
    status("fail", changed ? `${err.message}\nThe configuration changed outside this page, so the page now shows it as it is.`
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

function render({ focus = false } = {}) {
  const name = currentView();
  const view = VIEWS[name];
  document.title = `${view.title} · GitPersona`;
  for (const link of document.querySelectorAll(".nav a")) {
    if (link.getAttribute("href") === `#${name}`) {
      link.setAttribute("aria-current", "page");
    } else {
      link.removeAttribute("aria-current");
    }
  }
  const heading = h("h2", { tabindex: "-1" }, view.title);
  const section = h("section", { "aria-labelledby": "view-title" }, heading);
  heading.id = "view-title";
  if (view.needsConfig && !state.config_exists) {
    section.append(h("p", { class: "empty" }, "No configuration yet. ",
      h("a", { href: "#setup" }, "Set GitPersona up"), " first; it takes a few seconds."));
  } else {
    view.render(section, { state, run, refresh: refreshAll });
  }
  replace(document.getElementById("view"), section);
  if (focus) heading.focus();
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
  document.querySelector(".nav")?.remove();
  document.getElementById("close")?.remove();
  replace(document.getElementById("view"),
    h("section", {}, h("h2", {}, "GitPersona has stopped"), h("p", {}, "You can close this tab. Run `gitpersona ui` to open the page again.")));
  clearStatus();
}

async function start() {
  document.getElementById("close").addEventListener("click", closePage);
  window.addEventListener("hashchange", () => {
    if (closed) return;
    clearStatus();
    render({ focus: true });
  });
  try {
    await loadState();
    render();
  } catch (err) {
    replace(document.getElementById("view"), h("div", { class: "notice fail", role: "alert" }, err.message));
  }
}

start();
