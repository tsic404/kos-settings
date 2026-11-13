// The settings page document. Every function here is pure: it takes the page
// projection the control plane answers and returns the HTML the browser shows,
// so the page is judged without a DOM (KOSD-C-53). An item keeps the source
// type that judges it, its current value, and either the modification entry or
// the reason it has none (SETTINGS-C-36).

const SOURCE_LABELS = {
  object: "对象源",
  computed: "计算源",
  metric: "指标源",
  probe: "探测源",
  read: "读取源",
  local: "本地源",
  database: "数据库源",
};

const GROUPS = [
  { id: "cluster", title: "集群组" },
  { id: "node", title: "节点组" },
  { id: "other", title: "其他" },
];

const ACTION_LABELS = {
  "set-cluster-maintenance-window": "写入维护窗口",
  "start-node-maintenance": "发起维护",
  "restore-node-maintenance": "恢复维护",
};

const NODE_FIELD = "node";
const DOWNTIME_FIELD = "downtime";
const EMPTY = "暂无数据";
const NO_ENTRY_BY_ROLE = "当前角色只读，无修改入口";
const NO_ENTRY = "该项只读，无修改入口";
const CHANGE = "修改";
const SAVE = "保存";

// escapeHtml encodes a value for text and attribute positions alike, so a
// cluster object value can never inject markup into the page.
export function escapeHtml(value) {
  return String(value === null || value === undefined ? "" : value).replace(
    /[&<>"']/g,
    (character) => {
      switch (character) {
        case "&":
          return "&amp;";
        case "<":
          return "&lt;";
        case ">":
          return "&gt;";
        case '"':
          return "&quot;";
        default:
          return "&#39;";
      }
    },
  );
}

// sourceLabel names one of the seven source types (KOSD-C-53).
export function sourceLabel(source) {
  return SOURCE_LABELS[source] || source || "未知来源";
}

// groupOf places one item by the scope its identity declares: the product
// declares cluster-scoped items as cluster-<key> and node-scoped ones as
// node-<key>, and an identity outside both stays visible under 其他.
export function groupOf(id) {
  const identity = String(id || "");
  if (identity.startsWith("cluster-")) return "cluster";
  if (identity.startsWith("node-")) return "node";
  return "other";
}

// formatValue renders one item's current value as text. A per-node item is a
// list of entries; a record is rendered as key=value pairs, and a record that
// declares itself unavailable states the reason instead of a value (C-71).
export function formatValue(value) {
  if (value === null || value === undefined || value === "") return EMPTY;
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "string" || typeof value === "number") return String(value);
  if (Array.isArray(value)) {
    return value.length === 0 ? EMPTY : value.map(formatEntry).join("；");
  }
  if (typeof value === "object") {
    if (value.available === false) {
      return `不可用：${value.reason || "该项当前不可读"}`;
    }
    return pairsOf(value);
  }
  return String(value);
}

function pairsOf(record) {
  return Object.entries(record)
    .map(([key, value]) => `${key}=${scalarOf(value)}`)
    .join("；");
}

function scalarOf(value) {
  if (value === null || value === undefined) return EMPTY;
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) {
    return value.length === 0 ? EMPTY : value.map(scalarOf).join("、");
  }
  if (typeof value === "object") return pairsOf(value);
  return String(value);
}

function formatEntry(entry) {
  if (entry === null || typeof entry !== "object" || Array.isArray(entry)) {
    return scalarOf(entry);
  }
  if (typeof entry.name !== "string") return pairsOf(entry);
  const rest = Object.entries(entry).filter(([key]) => key !== "name");
  if (rest.length === 0) return entry.name;
  return `${entry.name}：${rest.map(([key, value]) => `${key}=${scalarOf(value)}`).join("；")}`;
}

// nodeNames reads the node identities the page already carries, so the
// maintenance entries offer the nodes the cluster reports rather than a list
// this module keeps on its own.
export function nodeNames(page) {
  const names = [];
  for (const item of page.items || []) {
    if (!Array.isArray(item.value)) continue;
    for (const entry of item.value) {
      if (!entry || typeof entry.name !== "string" || names.includes(entry.name)) continue;
      names.push(entry.name);
    }
  }
  return names;
}

// entryPath fills the placeholders a declared write path carries with the
// values the browser collected, encoding them so an identity cannot change the
// path or add its own query parameter.
export function entryPath(path, values) {
  let filled = path || "";
  for (const [name, value] of Object.entries(values || {})) {
    filled = filled.split(`{${name}}`).join(encodeURIComponent(value));
  }
  return filled;
}

// renderSettingsPage renders the whole settings page: the cluster and node
// groups item by item, then the page actions the role's projection declares.
export function renderSettingsPage(page) {
  const items = Array.isArray(page.items) ? page.items : [];
  const counts = new Map(GROUPS.map((group) => [group.id, 0]));
  for (const item of items) {
    const group = groupOf(item.id);
    counts.set(group, counts.get(group) + 1);
  }
  const sections = GROUPS.map((group) => {
    const members = items.filter((item) => groupOf(item.id) === group.id);
    if (members.length === 0) return "";
    return `
<section class="settings-group" data-group="${group.id}">
  <h2 class="group-title">${escapeHtml(group.title)}<span class="group-count">${members.length} 项</span></h2>
  ${members.map(renderItem).join("")}
</section>`;
  }).join("");
  const summary = GROUPS.filter((group) => counts.get(group.id) > 0)
    .map((group) => `${group.title} ${counts.get(group.id)} 项`)
    .join(" · ");
  const banner = page.banner ? renderBanner(page.banner) : "";
  const readOnly = page.readOnly
    ? `<p class="page-readonly" data-page-readonly>只读：${escapeHtml(page.readOnlyReason || "本页只提供读取")}</p>`
    : "";
  const actions = page.readOnly ? "" : renderPageActions(page, items);
  return `<section class="settings-page" data-page="settings">
  <header class="page-head">
    <h1>${escapeHtml(page.title || "系统设置")}</h1>
    <p class="page-summary" data-summary>${escapeHtml(summary)}</p>
  </header>
  ${banner}
  ${readOnly}
  ${sections}
  ${actions}
</section>`;
}

export function renderBanner(banner) {
  const message = banner.message || banner.reason || "";
  if (!message) return "";
  return `<p class="banner" data-banner>${escapeHtml(message)}</p>`;
}

// renderItem renders one item: its label, the source type that judges it, the
// current value, and exactly one of the modification entry or the reason no
// entry exists. An item the role cannot modify carries an action but no entry,
// which reads as read-only rather than as an error (KOSD-C-59).
function renderItem(item) {
  const source = item.source || "";
  const entry = item.entry;
  const display = formatValue(item.value);
  const stale = item.stale ? `<span class="stale" data-stale>数据已陈旧</span>` : "";
  let access = "";
  if (entry) {
    access = renderEntry(item);
  } else if (item.reason) {
    access = `<p class="item-reason" data-reason>${escapeHtml(item.reason)}</p>`;
  } else {
    access = `<p class="item-readonly" data-readonly>${item.action ? NO_ENTRY_BY_ROLE : NO_ENTRY}</p>`;
  }
  return `
<article class="setting-item" data-item="${escapeHtml(item.id)}" data-source="${escapeHtml(source)}" data-modifiable="${entry ? "true" : "false"}">
  <div class="item-head">
    <h3 class="item-label">${escapeHtml(item.label || item.id)}</h3>
    <span class="item-source" data-source-label="${escapeHtml(source)}">来源类型：${escapeHtml(sourceLabel(source))}</span>
  </div>
  <p class="item-value" data-value>${escapeHtml(display)}</p>
  <div class="item-access">${stale}${access}</div>
</article>`;
}

// renderEntry renders the modification entry the projection carries. A
// database-backed value is editable in place; any other entry is a write the
// browser submits unchanged.
function renderEntry(item) {
  const entry = item.entry;
  const method = escapeHtml(entry.method || "");
  const path = escapeHtml(entry.path || "");
  const action = escapeHtml(item.action || "");
  const identity = escapeHtml(item.id);
  if (item.source === "database") {
    const current = typeof item.value === "string" ? item.value : formatValue(item.value);
    return `<form class="item-entry" data-form="item" data-item="${identity}" data-action="${action}" data-method="${method}" data-path="${path}">
  <label class="item-field">当前值<input type="text" name="value" value="${escapeHtml(current)}"></label>
  <button type="submit">${SAVE}</button>
</form>`;
  }
  return `<button type="button" class="item-entry" data-entry data-item="${identity}" data-action="${action}" data-method="${method}" data-path="${path}">${CHANGE}</button>`;
}

// renderPageActions renders the declared actions no item carries, such as the
// node maintenance start and restore entries. The node and downtime controls
// fill the placeholders the declared path carries; a role whose projection
// declares no action receives no entry at all (KOSD-C-59).
function renderPageActions(page, items) {
  const bound = new Set(items.map((item) => item.action).filter(Boolean));
  const actions = (Array.isArray(page.actions) ? page.actions : []).filter(
    (action) => action && action.id && !bound.has(action.id),
  );
  if (actions.length === 0) return "";
  const names = nodeNames(page);
  const needsNode = actions.some((action) => actionPath(action).includes(`{${NODE_FIELD}}`));
  const needsDowntime = actions.some((action) => actionPath(action).includes(`{${DOWNTIME_FIELD}}`));
  const nodeControl = needsNode ? renderNodeControl(names) : "";
  const downtimeControl = needsDowntime
    ? `<label class="item-field">预计停机（如 15m，留空即不声明）<input type="text" name="downtime" data-field="${DOWNTIME_FIELD}" placeholder="15m"></label>`
    : "";
  const buttons = actions
    .map(
      (action) =>
        `<button type="button" class="page-action" data-action-id="${escapeHtml(action.id)}" data-method="${escapeHtml(action.entry && action.entry.method ? action.entry.method : "")}" data-path="${escapeHtml(actionPath(action))}">${escapeHtml(ACTION_LABELS[action.id] || action.id)}</button>`,
    )
    .join("");
  return `
<section class="page-actions" data-page-actions>
  <h2 class="group-title">维护操作</h2>
  <div class="action-controls">
    ${nodeControl}
    ${downtimeControl}
    ${buttons}
  </div>
  <p class="action-note" data-action-note>维护发起与恢复仅对 Owner 提供入口。</p>
</section>`;
}

function renderNodeControl(names) {
  if (names.length === 0) {
    return `<label class="item-field">节点<input type="text" name="node" data-field="${NODE_FIELD}" required></label>`;
  }
  const options = names
    .map((name) => `<option value="${escapeHtml(name)}">${escapeHtml(name)}</option>`)
    .join("");
  return `<label class="item-field">节点<select name="node" data-field="${NODE_FIELD}">${options}</select></label>`;
}

function actionPath(action) {
  return action && action.entry && action.entry.path ? action.entry.path : "";
}
