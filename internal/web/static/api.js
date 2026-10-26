import { lang, t } from "./i18n.js";

// Calls the local GitPersona API. Every change carries the page's CSRF token;
// the browser adds the Origin header, which the server also checks.

const csrfToken = document.querySelector('meta[name="csrf-token"]')?.content ?? "";



export class ApiError extends Error {
  constructor(message, log = "") {
    super(message);
    this.name = "ApiError";
    this.log = log;
  }
}

// request returns {data, log} or throws ApiError with the server's message.
export async function request(method, path, body) {
  // GitPersona answers in the page's language (health checks, findings, errors).
  const init = { method, headers: { "X-GitPersona-Lang": lang() }, credentials: "same-origin" };
  if (method !== "GET") {
    init.headers["X-GitPersona-CSRF"] = csrfToken;
  }
  if (body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  let response;
  try {
    response = await fetch(path, init);
  } catch {
    throw new ApiError(t("api.stopped"));
  }
  let envelope;
  try {
    envelope = await response.json();
  } catch {
    if (response.status === 401 || response.status === 403) {
      throw new ApiError(t("api.session"));
    }
    throw new ApiError(t("api.unexpected", { status: response.status }));
  }
  if (!envelope.ok) {
    throw new ApiError(envelope.error || t("api.failed"), envelope.log || "");
  }
  return { data: envelope.data, log: envelope.log || "" };
}

export const api = {
  // fresh asks gh about the accounts again instead of reusing its last answer.
  state: (fresh = false) => request("GET", fresh ? "/api/state?fresh=1" : "/api/state"),
  preview: () => request("GET", "/api/preview"),
  // plan is the hash of the preview the user saw; the server refuses another.
  apply: (yes, plan) => request("POST", "/api/apply", { yes, plan }),
  proposeSetup: () => request("GET", "/api/setup"),
  setupPreview: (plan) => request("POST", "/api/setup/preview", plan),
  setupApply: (plan) => request("POST", "/api/setup", plan),
  discover: (roots) => request("GET", "/api/discover" + (roots ? "?roots=" + encodeURIComponent(roots) : "")),
  resolve: (dir) => request("GET", "/api/resolve?dir=" + encodeURIComponent(dir)),
  // auto is the overview's own check: capped, and reused while nothing changed.
  verify: (dirs, ssh, auto = false) => request("POST", "/api/verify", { dirs, ssh, auto }),
  normalizePaths: (paths) => request("POST", "/api/paths", { paths }),
  addProfile: (profile) => request("POST", "/api/profiles", profile),
  updateProfile: (id, changes) => request("PATCH", "/api/profiles/" + encodeURIComponent(id), changes),
  removeProfile: (id) => request("DELETE", "/api/profiles/" + encodeURIComponent(id)),
  setDefault: (id) => request("POST", "/api/default", { id }),
  addDir: (dir, profile) => request("POST", "/api/dirs", { dir, profile }),
  removeDir: (dir) => request("DELETE", "/api/dirs", { dir }),
  addPin: (repo, profile) => request("POST", "/api/pins", { repo, profile }),
  removePin: (repo) => request("DELETE", "/api/pins", { repo }),
  backups: () => request("GET", "/api/backups"),
  rollback: (name) => request("POST", "/api/rollback", { name }),
  removeOldRules: () => request("POST", "/api/old-rules", {}),
  previewShells: (shells) => request("POST", "/api/shells/preview", { shells }),
  installShells: (shells) => request("POST", "/api/shells", { shells }),
  close: () => request("POST", "/api/close", {}),
};

// normalizePath writes a path the way the configuration stores it (home as
// ~, no trailing slash), so what the user typed compares and shows like the
// stored paths. On failure the path is kept; the change itself reports why.
export async function normalizePath(path) {
  try {
    return (await api.normalizePaths([path])).data[0] ?? path;
  } catch {
    return path;
  }
}
