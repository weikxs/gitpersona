import { t } from "./i18n.js";

// Small DOM helpers. Text is always set with textContent, never parsed as
// HTML, so values from the configuration cannot inject markup.

// h creates an element: h("p", {class: "muted"}, "text", child).
// Attributes starting with "on" are event listeners; true/false toggle
// boolean attributes; null and undefined are skipped.
export function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [name, value] of Object.entries(attrs)) {
    if (value === null || value === undefined || value === false) continue;
    if (name.startsWith("on") && typeof value === "function") {
      el.addEventListener(name.slice(2).toLowerCase(), value);
    } else if (value === true) {
      el.setAttribute(name, "");
    } else {
      el.setAttribute(name, String(value));
    }
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    el.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
}

export function replace(el, ...children) {
  el.replaceChildren();
  append(el, children);
}

// badge shows a status as an icon character and a word, not color alone.
const BADGES = { ok: ["✓", "badge.ok"], warn: ["!", "badge.warn"], fail: ["✕", "badge.fail"], info: ["•", ""] };

export function badge(status, text) {
  const [icon, word] = BADGES[status] ?? BADGES.info;
  return h("span", { class: `badge ${status}` }, h("span", { "aria-hidden": "true" }, icon), text ?? (word ? t(word) : ""));
}

// field builds a labelled input with optional hint text.
let fieldCount = 0;
export function field(label, input, hint) {
  const id = input.id || `f${++fieldCount}`;
  input.id = id;
  const children = [h("label", { for: id }, label), input];
  if (hint) {
    const hintId = `${id}-hint`;
    input.setAttribute("aria-describedby", hintId);
    children.push(h("span", { class: "hint", id: hintId }, hint));
  }
  return h("div", { class: "field" }, children);
}

// status shows a message in the live region, with any command output.
export function status(kind, message, log = "") {
  const region = document.getElementById("status");
  const children = [h("div", { class: `notice ${kind}`, tabindex: "-1" }, message, log ? h("pre", {}, log.trim()) : null)];
  replace(region, children);
}

export function clearStatus() {
  replace(document.getElementById("status"));
}

// confirm asks before a destructive action; resolves true when confirmed.
export function confirm(title, text, action = t("dialog.confirm")) {
  const dialog = document.getElementById("confirm");
  document.getElementById("confirm-title").textContent = title;
  document.getElementById("confirm-text").textContent = text;
  document.getElementById("confirm-ok").textContent = action;
  document.getElementById("confirm-cancel").textContent = t("dialog.cancel");
  return new Promise((resolve) => {
    dialog.addEventListener("close", () => resolve(dialog.returnValue === "ok"), { once: true });
    dialog.returnValue = "";
    dialog.showModal();
  });
}

// busy disables a button while work runs, so it cannot be pressed twice.
// Afterwards focus returns to the button when it is still on the page and
// focus was lost (for example when a confirmation was cancelled).
export async function busy(button, work) {
  button.setAttribute("aria-busy", "true");
  button.disabled = true;
  try {
    return await work();
  } finally {
    button.removeAttribute("aria-busy");
    button.disabled = false;
    if (button.isConnected && (!document.activeElement || document.activeElement === document.body)) button.focus();
  }
}
