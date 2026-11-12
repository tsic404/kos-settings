// Package settingstc62 judges the settings page SETTINGS-C-36 declares: the
// cluster group and the node group item by item with their current value and
// source type, the one modification entry and the reason every unmodifiable
// item carries, and the Owner-only maintenance entries (SETTINGS-AC-31,
// SETTINGS-TC-62, KOSD-C-59).

import test from "node:test";
import assert from "node:assert/strict";

import { entryPath, renderSettingsPage, sourceLabel } from "../../../assets/settings/render.js";
import { NODE_CONFIGURATION_REASON, memberPage, ownerPage } from "./page.fixtures.js";

// groupSection returns one group's rendered section.
function groupSection(html, id) {
  const start = html.indexOf(`data-group="${id}"`);
  if (start < 0) return "";
  const end = html.indexOf("</section>", start);
  return html.slice(start, end < 0 ? undefined : end);
}

// articleOf returns one item's rendered article.
function articleOf(html, id) {
  const marker = `data-item="${id}"`;
  const start = html.indexOf(marker);
  if (start < 0) return "";
  const open = html.lastIndexOf("<article", start);
  const close = html.indexOf("</article>", start);
  return html.slice(open, close < 0 ? undefined : close);
}

// actionsSection returns the page actions section.
function actionsSection(html) {
  const start = html.indexOf("data-page-actions");
  if (start < 0) return "";
  return html.slice(html.lastIndexOf("<section", start), html.indexOf("</section>", start));
}

function countOf(html, needle) {
  return html.split(needle).length - 1;
}

function valueOf(article) {
  const matched = article.match(/<p class="item-value" data-value>([\s\S]*?)<\/p>/);
  return matched ? matched[1].trim() : "";
}

test("设置页渲染集群组 9 项与节点组 11 项，逐项展示当前值与来源类型", () => {
  const html = renderSettingsPage(ownerPage);
  assert.equal(countOf(groupSection(html, "cluster"), '<article class="setting-item"'), 9);
  assert.equal(countOf(groupSection(html, "node"), '<article class="setting-item"'), 11);
  assert.equal(countOf(html, '<article class="setting-item"'), 20);

  for (const entry of ownerPage.items) {
    const article = articleOf(html, entry.id);
    assert.notEqual(article, "", `条目 ${entry.id} 未渲染`);
    assert.match(article, new RegExp(`data-source="${entry.source}"`), `条目 ${entry.id} 未带来源类型`);
    assert.ok(
      article.includes(`来源类型：${sourceLabel(entry.source)}`),
      `条目 ${entry.id} 未显示可读的来源类型`,
    );
    assert.notEqual(valueOf(article), "", `条目 ${entry.id} 未显示当前值`);
  }

  assert.ok(articleOf(html, "cluster-version").includes("v1.37.0"), "集群版本未显示当前值");
  assert.ok(
    articleOf(html, "cluster-maintenance-window").includes("Sun 02:00-04:00"),
    "集群策略未显示数据库中的当前值",
  );
  assert.ok(
    articleOf(html, "node-status").includes("node-b：ready=是；schedulable=否"),
    "节点状态未显示节点的当前值",
  );
});

test("不可修改项给出原因且不提供修改入口", () => {
  const html = renderSettingsPage(ownerPage);
  const facts = ownerPage.items.filter((entry) => !entry.entry);
  assert.equal(facts.length, 19);
  for (const entry of facts) {
    const article = articleOf(html, entry.id);
    assert.equal(article.includes('data-form="item"'), false, `条目 ${entry.id} 提供了表单入口`);
    assert.equal(article.includes("data-entry"), false, `条目 ${entry.id} 提供了修改入口`);
    assert.match(article, /data-reason/, `条目 ${entry.id} 没有给出原因`);
    assert.ok(article.includes(entry.reason), `条目 ${entry.id} 的原因不可读`);
  }
  assert.ok(
    articleOf(html, "node-level-configuration").includes(NODE_CONFIGURATION_REASON),
    "节点级配置没有给出不可修改的原因",
  );
});

test("Owner 可见维护窗口写入与维护发起、恢复入口", () => {
  const html = renderSettingsPage(ownerPage);
  const window = articleOf(html, "cluster-maintenance-window");
  assert.match(window, /data-form="item"/, "维护窗口没有写入入口");
  assert.match(window, /data-method="PUT"/, "维护窗口入口不是控制面写入");
  assert.match(window, /data-path="\/api\/v1\/settings\/cluster-maintenance-window"/);

  const actions = actionsSection(html);
  assert.match(actions, /data-action-id="start-node-maintenance"/, "缺少维护发起入口");
  assert.match(actions, /data-method="POST"/);
  assert.ok(actions.includes(`data-path="/api/v1/node-maintenance?node={node}&amp;downtime={downtime}"`));
  assert.ok(actions.includes("发起维护"));
  assert.match(actions, /data-action-id="restore-node-maintenance"/, "缺少维护恢复入口");
  assert.match(actions, /data-method="DELETE"/);
  assert.ok(actions.includes('data-path="/api/v1/node-maintenance/{node}"'));
  assert.ok(actions.includes("恢复维护"));

  // A window write appears once: the item entry, not a second page action.
  assert.equal(countOf(html, 'data-path="/api/v1/settings/cluster-maintenance-window"'), 1);
});

test("维护入口按声明的占位符填入节点与停机时间", () => {
  assert.equal(
    entryPath("/api/v1/node-maintenance?node={node}&downtime={downtime}", { node: "node-a", downtime: "30m" }),
    "/api/v1/node-maintenance?node=node-a&downtime=30m",
  );
  assert.equal(
    entryPath("/api/v1/node-maintenance/{node}", { node: "node/a b" }),
    "/api/v1/node-maintenance/node%2Fa%20b",
    "节点名未按路径段编码",
  );
  assert.equal(entryPath("/api/v1/settings/cluster-maintenance-window", {}), "/api/v1/settings/cluster-maintenance-window");
});

test("Member 读取同样的 20 项，但没有任何修改入口", () => {
  const html = renderSettingsPage(memberPage);
  assert.equal(countOf(html, '<article class="setting-item"'), 20);
  assert.equal(countOf(html, 'data-form="item"'), 0);
  assert.equal(countOf(html, "data-entry"), 0);
  assert.equal(countOf(html, "data-action-id"), 0);
  assert.equal(countOf(html, "data-page-actions"), 0);

  const window = articleOf(html, "cluster-maintenance-window");
  assert.match(window, /data-readonly/, "Member 的策略项未标为只读");
  assert.ok(window.includes("当前角色只读"), "Member 的策略项未说明只读");
  assert.ok(window.includes("Sun 02:00-04:00"), "Member 读取不到策略的当前值");
});

test("每个条目只有一个来源类型，标签与 KOSD-C-53 的七类一致", () => {
  const html = renderSettingsPage(ownerPage);
  const seen = new Map([
    ["object", "对象源"],
    ["computed", "计算源"],
    ["metric", "指标源"],
    ["probe", "探测源"],
    ["read", "读取源"],
    ["local", "本地源"],
    ["database", "数据库源"],
  ]);
  for (const entry of ownerPage.items) {
    const article = articleOf(html, entry.id);
    assert.equal(countOf(article, 'data-source="'), 1, `条目 ${entry.id} 出现多种来源类型`);
    assert.equal(sourceLabel(entry.source), seen.get(entry.source));
  }
});

test("指标源不可用时给出原因，容量不被呈现为用量", () => {
  const article = articleOf(renderSettingsPage(ownerPage), "cluster-usage");
  assert.ok(article.includes("不可用：发行版没有接入资源指标来源"), "用量项未给出不可用原因");
  assert.equal(article.includes("cpu="), false, "用量项呈现了容量");
});

test("集群对象的值按文本渲染，不把标记注入页面", () => {
  const hostile = structuredClone(ownerPage);
  hostile.items[0].label = '<img src=x onerror="alert(1)">';
  hostile.items[0].value[0].roles = ["<script>alert(1)</script>"];
  const html = renderSettingsPage(hostile);
  assert.equal(html.includes("<img"), false, "标签被原样注入");
  assert.equal(html.includes("<script>alert"), false, "值被原样注入");
  assert.ok(html.includes("&lt;img"), "标签未被转义");
  assert.ok(html.includes("&lt;script&gt;"), "值未被转义");
});
