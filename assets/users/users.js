// Browser entry of the user page: read the page projection, render it, and
// submit the writes it declares. The browser sends its own session with every
// write; this module never holds a cluster credential (KOSD-F-9).

import { renderUsersPage } from "./render-users.js";
import { ownerBody, visibilityBody } from "./write-body.js";

const PAGE_PATH = "/api/v1/pages/users";
const ROOT_ID = "users-root";
const RETRY = "重试";
const LOADING = "正在读取用户…";
const READ_FAILED = "用户页读取失败";
const WRITE_FAILED = "写入失败";

function root() {
  return document.getElementById(ROOT_ID);
}

function failureHtml(failure, fallback) {
  const code = failure && failure.code ? failure.code : "internal";
  const message = failure && failure.message ? failure.message : fallback;
  return `<div class="failure" data-failure="${escapeAttribute(code)}"><p>${escapeAttribute(message)}</p><button type="button" data-action="reload">${RETRY}</button></div>`;
}

function escapeAttribute(value) {
  return String(value === null || value === undefined ? "" : value).replace(
    /[&<>"']/g,
    (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character],
  );
}

async function answer(response) {
  const body = await response.json().catch(() => null);
  if (response.ok) return body;
  throw body || { code: "internal", message: `${WRITE_FAILED}（${response.status}）` };
}

async function load() {
  const target = root();
  if (!target) return;
  target.dataset.state = "loading";
  target.innerHTML = `<p class="loading" data-state="loading">${LOADING}</p>`;
  try {
    const response = await fetch(PAGE_PATH, {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    const page = await answer(response);
    target.dataset.state = "ready";
    target.innerHTML = renderUsersPage(page);
  } catch (failure) {
    target.dataset.state = "error";
    target.innerHTML = failureHtml(failure, READ_FAILED);
  }
}

// objectPath names the control-plane read of the object one entry writes, so a
// write carries the stored document's other fields instead of replacing them.
function objectPath(form) {
  const kind = encodeURIComponent(form.dataset.kind || "");
  const name = encodeURIComponent(form.dataset.name || "");
  const namespace = encodeURIComponent(form.dataset.namespace || "");
  return `/api/v1/resources/${kind}/${name}?namespace=${namespace}`;
}

async function storedObject(form) {
  const response = await fetch(objectPath(form), {
    headers: { Accept: "application/json" },
    credentials: "same-origin",
  });
  return answer(response);
}

async function submit(form) {
  const method = form.dataset.method || "PUT";
  const path = form.dataset.path;
  let body;
  switch (form.dataset.form) {
    case "member-role": {
      body = { role: new FormData(form).get("role") || "" };
      break;
    }
    case "app-visibility": {
      const stored = await storedObject(form);
      const fields = new FormData(form);
      body = visibilityBody(stored, form.dataset.name, fields.get("members"), fields.get("allowGuests") === "on");
      break;
    }
    case "transfer-owner": {
      const stored = await storedObject(form);
      body = ownerBody(stored, form.dataset.name, new FormData(form).get("owner"));
      break;
    }
    default:
      return;
  }
  const response = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(body),
  });
  await answer(response);
  await load();
}

function report(failure) {
  const target = root();
  if (!target) return;
  const existing = target.querySelector("[data-failure]");
  if (existing) existing.remove();
  target.insertAdjacentHTML("afterbegin", failureHtml(failure, WRITE_FAILED));
}

document.addEventListener("submit", (event) => {
  const form = event.target.closest("form[data-entry]");
  if (!form) return;
  event.preventDefault();
  submit(form).catch(report);
});

document.addEventListener("click", (event) => {
  if (event.target.closest('[data-action="reload"]')) load();
});

load();
