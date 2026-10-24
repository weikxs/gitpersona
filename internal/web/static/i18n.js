// Interface text in English and Simplified Chinese. The page starts in the
// system's language, else the browser's, else English; a language picked
// with the switch lasts while this tab is open. Messages from
// GitPersona itself (health checks, findings, errors) come translated from
// the server, which reads the language from every request.

import { en } from "./i18n-en.js";
import { zh } from "./i18n-zh.js";

const DICTIONARIES = { en, zh };
const STORE_KEY = "gitpersona-lang";

export const LANGUAGES = [["zh", "中文"], ["en", "EN"]];

let current = initial();

function initial() {
  try {
    const picked = sessionStorage.getItem(STORE_KEY);
    if (picked && DICTIONARIES[picked]) return picked;
  } catch {
    // Storage may be unavailable; the system or browser decides.
  }
  // GitPersona reads the system's language when it serves the page; it is
  // empty when the system uses a language the page does not have.
  const system = document.querySelector('meta[name="system-lang"]')?.content ?? "";
  if (DICTIONARIES[system]) return system;
  const preferred = navigator.languages?.length ? navigator.languages : [navigator.language || ""];
  for (const tag of preferred) {
    if (/^zh\b/i.test(tag)) return "zh";
    if (/^en\b/i.test(tag)) return "en";
  }
  return "en";
}

export function lang() {
  return current;
}

// setLang switches the language for as long as this tab is open; the next
// page follows the system again.
export function setLang(next) {
  if (!DICTIONARIES[next]) return;
  current = next;
  try {
    sessionStorage.setItem(STORE_KEY, next);
  } catch {
    // The choice then lasts until the page is reloaded.
  }
  applyDocumentLang();
}

export function applyDocumentLang() {
  document.documentElement.lang = current === "zh" ? "zh-CN" : "en";
}

// t returns the text for key, with {name} placeholders filled from params.
export function t(key, params = {}) {
  const text = DICTIONARIES[current][key] ?? en[key] ?? key;
  return text.replace(/\{(\w+)\}/g, (match, name) => (name in params ? String(params[name]) : match));
}

// tn picks the singular key when n is 1 and the plural key otherwise; n is
// available to both as {n}.
export function tn(n, one, many, params = {}) {
  return t(n === 1 ? one : many, { n, ...params });
}

// dateTime formats a date in the page's language.
export function dateTime(date) {
  return new Intl.DateTimeFormat(current === "zh" ? "zh-CN" : "en", { dateStyle: "medium", timeStyle: "medium" }).format(date);
}
