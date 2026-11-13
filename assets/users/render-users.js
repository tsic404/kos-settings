// The user page document SETTINGS-C-1 declares. Every function here is pure: it
// takes the page projection the control plane answers and returns the HTML the
// browser shows, so the page is judged without a DOM (KOSD-C-53). Each item
// keeps the source type that judges it, its current value, and exactly one of
// the modification entry or the reason it has none (C-1, KOSD-C-59).

import { escapeHtml, entryPath, formatValue, renderBanner, sourceLabel } from "../settings/render.js";
import { joinMembers } from "./write-body.js";

const ROLES = ["Owner", "Member", "Guest"];
const ROLE_FIELD = "role";
const OWNER_FIELD = "owner";
const MEMBERS_FIELD = "members";
const MEMBERS_DISPLAY_SEPARATOR = "、";
const ALLOW_GUESTS_FIELD = "allowGuests";
const READONLY_BY_ROLE = "当前角色只读，无修改入口";
const READONLY_BY_OWNER = "只读：仅应用所有者可修改";
const NO_ROWS = "该项当前没有可修改的行";
const READONLY = "该项只读，无修改入口";
const SAVE = "保存";
const ROLE_NOTE = "成员与角色、应用与卷所有者只由 Owner 写入；应用可见性按 KOSD-C-59 的所有者规则写入。";

// isWritable judges one item: an entry changes something when the projection
// carries one and at least one shown row is writable for this session. The
// visibility entry is declared for every role the page admits and each row
// decides, so a guest and a member owning no application see it as read-only
// (KOSD-C-59).
function isWritable(item) {
  if (!item.entry) return false;
  const rows = Array.isArray(item.value) ? item.value : [];
  return rows.some((row) => !row || row.writable !== false);
}

// renderUsersPage renders the whole user page: the three items SETTINGS-C-1
// declares, each with its source type, value and access state.
export function renderUsersPage(page) {
  const items = Array.isArray(page.items) ? page.items : [];
  const banner = page.banner ? renderBanner(page.banner) : "";
  const readOnly = page.readOnly
    ? `<p class="page-readonly" data-page-readonly>只读：${escapeHtml(page.readOnlyReason || "本页只提供读取")}</p>`
    : "";
  const note = items.some(isWritable) ? `<p class="action-note" data-role-note>${escapeHtml(ROLE_NOTE)}</p>` : "";
  return `<section class="users-page" data-page="users">
  <header class="page-head">
    <h1>${escapeHtml(page.title || "用户")}</h1>
    <p class="page-summary" data-summary>${escapeHtml(items.map((item) => item.label || item.id).join(" · "))}</p>
  </header>
  ${banner}
  ${readOnly}
  ${items.map(renderItem).join("")}
  ${note}
</section>`;
}

// renderItem renders one item: its label, the source type that judges it, the
// current value rows, and the modification entry or the reason it has none.
function renderItem(item) {
  const source = item.source || "";
  return `
<article class="setting-item" data-item="${escapeHtml(item.id)}" data-source="${escapeHtml(source)}" data-modifiable="${isWritable(item) ? "true" : "false"}">
  <div class="item-head">
    <h3 class="item-label">${escapeHtml(item.label || item.id)}</h3>
    <span class="item-source" data-source-label="${escapeHtml(source)}">来源类型：${escapeHtml(sourceLabel(source))}</span>
  </div>
  ${rowsOf(item)}
  <div class="item-access">${accessOf(item)}</div>
</article>`;
}

// accessOf renders exactly one of the modification entries or the reason no
// entry exists. An item the session may not write carries an action but no
// writable row, which reads as read-only rather than as an error (C-59).
function accessOf(item) {
  if (isWritable(item)) return "";
  if (item.reason) return `<p class="item-reason" data-reason>${escapeHtml(item.reason)}</p>`;
  if (!item.entry) return `<p class="item-readonly" data-readonly>${item.action ? READONLY_BY_ROLE : READONLY}</p>`;
  const rows = Array.isArray(item.value) ? item.value : [];
  const text = rows.length === 0 ? NO_ROWS : READONLY_BY_OWNER;
  return `<p class="item-readonly" data-readonly>${text}</p>`;
}

// rowsOf renders the value as the rows the item shows, or as one text value
// when the source states it cannot answer or the list is empty (C-1, C-71).
function rowsOf(item) {
  const value = item.value;
  if (!Array.isArray(value) || value.length === 0) {
    return `<p class="item-value" data-value>${escapeHtml(formatValue(value))}</p>`;
  }
  return `<ul class="item-rows">${value.map((row) => renderRow(item, row)).join("")}</ul>`;
}

// renderRow renders one row with the identity the entry fills and, when the
// projection carries the item's entry and the row is writable for this session,
// the write that changes it (KOSD-C-59).
function renderRow(item, row) {
  const kind = kindOf(row);
  const identity = identityOf(kind, row);
  const canWrite = Boolean(item.entry) && (!row || row.writable !== false);
  const entry = canWrite ? renderEntry(item, kind, row, identity) : "";
  const readOnly = item.entry && !canWrite && kind === "visibility"
    ? `<span class="row-readonly" data-row-readonly>${READONLY_BY_OWNER}</span>`
    : "";
  return `<li class="item-row" data-row="${escapeHtml(identity.label)}" data-row-kind="${escapeHtml(kind)}">
  <span class="row-label">${escapeHtml(identity.label)}</span>
  <span class="row-detail" data-row-detail>${escapeHtml(detailOf(kind, row))}</span>
  ${entry}${readOnly}
</li>`;
}

// kindOf names the row shape a value carries: one account, one application's
// visibility, or one owned object.
function kindOf(row) {
  if (row === null || typeof row !== "object") return "plain";
  if (typeof row.username === "string") return "member";
  if (typeof row.kind === "string") return "owner";
  if (typeof row.namespace === "string" && typeof row.name === "string") return "visibility";
  return "plain";
}

// identityOf returns the row's display label and the values its entry fills.
function identityOf(kind, row) {
  switch (kind) {
    case "member":
      return { label: row.username, values: { username: row.username } };
    case "owner":
      return {
        label: `${row.kind} ${row.namespace}/${row.name}`,
        values: { kind: row.kind, name: row.name, namespace: row.namespace },
      };
    case "visibility":
      return { label: `${row.namespace}/${row.name}`, values: { name: row.name, namespace: row.namespace } };
    default:
      return { label: formatValue(row), values: {} };
  }
}

// detailOf states the property the item is about: an account's role and state,
// an application's visibility, or an object's owner.
function detailOf(kind, row) {
  switch (kind) {
    case "member":
      return `角色：${row.role || "无角色"}${row.disabled ? "；已停用" : ""}`;
    case "visibility":
      return `成员：${membersOf(row)}；访客：${row.allowGuests ? "是" : "否"}`;
    case "owner":
      return `所有者：${row.owner || "无所有者"}`;
    default:
      return formatValue(row);
  }
}

// membersOf reads the names as prose; the write field keeps the same names one
// per line, because a login name may contain the display separator.
function membersOf(row) {
  const listed = Array.isArray(row.members) ? row.members : [];
  return listed.length === 0 ? "无列名成员" : listed.join(MEMBERS_DISPLAY_SEPARATOR);
}

// renderEntry renders the write the projection carries for one row. The path
// names the row identity the browser fills, and the form carries the fields the
// write changes; a role the projection gives no entry receives no control.
function renderEntry(item, kind, row, identity) {
  const method = escapeHtml(item.entry.method || "");
  const path = escapeHtml(entryPath(item.entry.path, identity.values));
  const identityAttributes = identityValues(kind, row);
  switch (kind) {
    case "member":
      return `<form class="row-entry" data-entry data-form="member-role" data-method="${method}" data-path="${path}" ${identityAttributes}>
  <label class="item-field">角色<select name="${ROLE_FIELD}">${ROLES.map(
    (role) => `<option value="${role}"${role === row.role ? " selected" : ""}>${role}</option>`,
  ).join("")}</select></label>
  <button type="submit">${SAVE}</button>
</form>`;
    case "visibility":
      return `<form class="row-entry" data-entry data-form="app-visibility" data-method="${method}" data-path="${path}" ${identityAttributes}>
  <label class="item-field">成员<textarea name="${MEMBERS_FIELD}" rows="2">${escapeHtml(joinMembers(row.members))}</textarea></label>
  <label class="item-field"><input type="checkbox" name="${ALLOW_GUESTS_FIELD}"${row.allowGuests ? " checked" : ""}>允许访客</label>
  <button type="submit">${SAVE}</button>
</form>`;
    case "owner":
      return `<form class="row-entry" data-entry data-form="transfer-owner" data-method="${method}" data-path="${path}" ${identityAttributes}>
  <label class="item-field">新所有者<input type="text" name="${OWNER_FIELD}" value="${escapeHtml(row.owner || "")}"></label>
  <button type="submit">${SAVE}</button>
</form>`;
    default:
      return "";
  }
}

// identityValues carries the row identity the browser needs to read the object
// it is about to write, so the write keeps the stored document's other fields.
function identityValues(kind, row) {
  switch (kind) {
    case "owner":
      return `data-kind="${escapeHtml(row.kind)}" data-namespace="${escapeHtml(row.namespace)}" data-name="${escapeHtml(row.name)}"`;
    case "visibility":
      return `data-kind="Application" data-namespace="${escapeHtml(row.namespace)}" data-name="${escapeHtml(row.name)}"`;
    case "member":
      return `data-username="${escapeHtml(row.username)}"`;
    default:
      return "";
  }
}
