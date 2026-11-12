// Browser entry of the settings page: read the page projection, render it, and
// submit the writes the projection declares. The browser sends its own session
// with every write; this module never holds a cluster credential (KOSD-F-9).

import { entryPath, escapeHtml, renderSettingsPage } from "./render.js";

const PAGE_PATH = "/api/v1/pages/settings";
const ROOT_ID = "settings-root";
const NODE_FIELD = "node";
const DOWNTIME_FIELD = "downtime";
const RETRY = "重试";
const LOADING = "正在读取设置…";
const READ_FAILED = "设置读取失败";
const WRITE_FAILED = "设置写入失败";

function root() {
  return document.getElementById(ROOT_ID);
}

function failureHtml(failure, fallback) {
  const code = failure && failure.code ? failure.code : "internal";
  const message = failure && failure.message ? failure.message : fallback;
  return `<div class="failure" data-failure="${escapeHtml(code)}"><p>${escapeHtml(message)}</p><button type="button" data-action="reload">${RETRY}</button></div>`;
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
    target.innerHTML = renderSettingsPage(page);
  } catch (failure) {
    target.dataset.state = "error";
    target.innerHTML = failureHtml(failure, READ_FAILED);
  }
}

function field(form, name) {
  const control = form.querySelector(`[data-field="${name}"]`);
  return control ? control.value : "";
}

async function submitItem(form) {
  const body = JSON.stringify({ value: new FormData(form).get("value") || "" });
  const response = await fetch(form.dataset.path, {
    method: form.dataset.method || "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    credentials: "same-origin",
    body,
  });
  await answer(response);
  await load();
}

async function submitAction(button) {
  const values = {
    [NODE_FIELD]: field(button.closest("[data-page-actions]") || document, NODE_FIELD),
    [DOWNTIME_FIELD]: field(button.closest("[data-page-actions]") || document, DOWNTIME_FIELD),
  };
  const response = await fetch(entryPath(button.dataset.path, values), {
    method: button.dataset.method || "POST",
    headers: { Accept: "application/json" },
    credentials: "same-origin",
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
  const form = event.target.closest('form[data-form="item"]');
  if (!form) return;
  event.preventDefault();
  submitItem(form).catch(report);
});

document.addEventListener("click", (event) => {
  const retry = event.target.closest('[data-action="reload"]');
  if (retry) {
    load();
    return;
  }
  const button = event.target.closest("[data-action-id]");
  if (!button) return;
  button.disabled = true;
  submitAction(button)
    .catch(report)
    .finally(() => {
      button.disabled = false;
    });
});

load();
