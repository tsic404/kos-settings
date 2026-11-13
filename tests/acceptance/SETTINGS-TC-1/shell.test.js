// Package settingstc1 judges the rendered user page SETTINGS-C-1 declares: the
// three items with their source type and current value, the per-row
// modification entries KOSD-C-59 and KOSD-C-60 allow, the read-only state of the
// rows and roles the contract gives no entry to, and the reason the member list
// states while the control-plane database is down (SETTINGS-AC-1, SETTINGS-TC-1,
// KOSD-C-59, KOSD-C-61).

import test from "node:test";
import assert from "node:assert/strict";

import { sourceLabel } from "../../../assets/settings/render.js";
import { renderUsersPage } from "../../../assets/users/render-users.js";
import { ownerBody, visibilityBody } from "../../../assets/users/write-body.js";
import {
  DATABASE_REASON,
  databaseDownPage,
  guestPage,
  memberOwnerPage,
  memberPage,
  ownerPage,
  projectionOf,
  storedApplication,
} from "./page.fixtures.js";

// articleOf returns one item's rendered article.
function articleOf(html, id) {
  const marker = `data-item="${id}"`;
  const start = html.indexOf(marker);
  if (start < 0) return "";
  const open = html.lastIndexOf("<article", start);
  const close = html.indexOf("</article>", start);
  return html.slice(open, close < 0 ? undefined : close);
}

function countOf(html, needle) {
  return html.split(needle).length - 1;
}

// membersValueOf returns the member field one row's entry is filled with: one
// login per line, since a login name cannot carry a line break.
function membersValueOf(html, path) {
  const start = html.indexOf(path);
  if (start < 0) return null;
  const match = /<textarea name="members"[^>]*>([^<]*)<\/textarea>/.exec(html.slice(start));
  return match ? match[1] : null;
}

test("用户页渲染三项，逐项给出来源类型与当前值", () => {
  const html = renderUsersPage(ownerPage);
  assert.equal(countOf(html, '<article class="setting-item"'), 3);

  for (const entry of ownerPage.items) {
    const article = articleOf(html, entry.id);
    assert.notEqual(article, "", `条目 ${entry.id} 未渲染`);
    assert.match(article, new RegExp(`data-source="${entry.source}"`), `条目 ${entry.id} 未带来源类型`);
    assert.ok(article.includes(`来源类型：${sourceLabel(entry.source)}`), `条目 ${entry.id} 未显示可读的来源类型`);
  }

  const members = articleOf(html, "users-members-and-roles");
  assert.ok(members.includes("alice") && members.includes("角色：Owner"), "成员与角色未显示账号与角色");
  assert.ok(members.includes("carol") && members.includes("已停用"), "成员与角色未显示停用状态");
  const visibility = articleOf(html, "users-app-visibility");
  assert.ok(visibility.includes("default/shop") && visibility.includes("成员：bob、carol"), "应用可见性未显示列名成员");
  assert.ok(visibility.includes("default/blog") && visibility.includes("访客：是"), "应用可见性未显示访客开关");
  const owners = articleOf(html, "users-app-and-volume-owners");
  assert.ok(owners.includes("Application default/shop") && owners.includes("所有者：bob"), "所有者项未显示应用所有者");
  assert.ok(owners.includes("Volume default/data") && owners.includes("所有者：alice"), "所有者项未显示卷所有者");
});

test("Owner 的每一项都有修改入口，入口逐行带上对象身份", () => {
  const html = renderUsersPage(ownerPage);
  for (const entry of ownerPage.items) {
    assert.match(articleOf(html, entry.id), /data-modifiable="true"/, `条目 ${entry.id} 不是可修改的`);
  }

  const members = articleOf(html, "users-members-and-roles");
  assert.equal(countOf(members, 'data-form="member-role"'), 3, "成员行未逐行给出角色入口");
  assert.ok(members.includes('data-path="/api/v1/accounts/alice"'), "角色入口未填入账号身份");

  const visibility = articleOf(html, "users-app-visibility");
  assert.equal(countOf(visibility, 'data-form="app-visibility"'), 2, "应用行未逐行给出可见性入口");
  assert.ok(
    visibility.includes('data-path="/api/v1/resources/Application/shop?namespace=default"'),
    "可见性入口未填入应用身份",
  );

  const owners = articleOf(html, "users-app-and-volume-owners");
  assert.equal(countOf(owners, 'data-form="transfer-owner"'), 3, "对象行未逐行给出转移入口");
  assert.ok(
    owners.includes('data-path="/api/v1/resources/Volume/data?namespace=default"'),
    "转移入口未填入对象身份",
  );

  for (const placeholder of ["{username}", "{name}", "{namespace}", "{kind}"]) {
    assert.ok(!html.includes(placeholder), `页面保留未填的占位符 ${placeholder}`);
  }
});

test("Member 只在自己拥有的应用行看到入口", () => {
  const html = renderUsersPage(memberOwnerPage);
  const visibility = articleOf(html, "users-app-visibility");
  assert.match(visibility, /data-modifiable="true"/, "拥有应用的 Member 未得到可修改的可见性项");
  assert.equal(countOf(visibility, 'data-form="app-visibility"'), 1, "可见性入口未按行所有者收敛");
  assert.ok(
    visibility.includes('data-path="/api/v1/resources/Application/shop?namespace=default"'),
    "可见性入口未落在拥有的应用行",
  );
  assert.ok(visibility.includes("data-row-readonly"), "他人拥有的应用行未说明只读");
  assert.ok(!visibility.includes("Application/shared"), "他人拥有的应用行给出了入口");

  const members = articleOf(html, "users-members-and-roles");
  assert.equal(countOf(members, "data-entry"), 0, "Member 得到账号修改入口");
  const owners = articleOf(html, "users-app-and-volume-owners");
  assert.equal(countOf(owners, "data-entry"), 0, "Member 得到转移入口");
  assert.ok(members.includes("当前角色只读，无修改入口"), "成员与角色未说明角色只读");
  assert.ok(owners.includes("当前角色只读，无修改入口"), "所有者项未说明角色只读");
});

test("Member 不拥有任何可见应用与 Guest 看不到修改入口，页面按角色只读", () => {
  for (const [name, page] of [
    ["Member", memberPage],
    ["Guest", guestPage],
  ]) {
    const html = renderUsersPage(page);
    assert.equal(countOf(html, "data-entry"), 0, `${name} 收到修改入口`);
    assert.equal(countOf(html, 'data-modifiable="true"'), 0, `${name} 的条目被标为可修改`);
    const visibility = articleOf(html, "users-app-visibility");
    assert.ok(visibility.includes("只读：仅应用所有者可修改"), `${name} 的可见性项未说明按所有者只读`);
    for (const id of ["users-members-and-roles", "users-app-and-volume-owners"]) {
      const article = articleOf(html, id);
      assert.match(article, /data-modifiable="false"/, `${name} 的条目 ${id} 未标为只读`);
      assert.ok(article.includes("当前角色只读，无修改入口"), `${name} 的条目 ${id} 未说明角色只读`);
      assert.ok(!article.includes("data-entry"), `${name} 的条目 ${id} 带上了修改入口`);
    }
  }
});

// roundTripMembers fills and saves one visibility row whose members include a
// login name, the way the browser does, and returns the members the write body
// carries.
function roundTripMembers(login) {
  const page = {
    ...ownerPage,
    items: ownerPage.items.map((entry) =>
      entry.id === "users-app-visibility"
        ? {
            ...entry,
            value: [{ namespace: "default", name: "shop", members: [login, "bob"], allowGuests: false, writable: true }],
          }
        : entry,
    ),
  };
  const html = renderUsersPage(page);
  const value = membersValueOf(html, 'data-path="/api/v1/resources/Application/shop?namespace=default"');
  assert.equal(value, `${login}\nbob`, "成员输入框的回填改动了登录名");
  const body = visibilityBody(projectionOf(storedApplication), "shop", value, false);
  return body.spec.visibility.members;
}

test("两成员的可见性行回填后直接保存，成员表不变", () => {
  assert.deepEqual(roundTripMembers("alice"), ["alice", "bob"], "保存把两个成员并成了一个名字");
});

test("含空格的登录名回填后保存不变", () => {
  assert.deepEqual(roundTripMembers("a b"), ["a b", "bob"], "保存按空白切开了登录名");
});

test("含逗号的登录名回填后保存不变", () => {
  assert.deepEqual(roundTripMembers("x,y"), ["x,y", "bob"], "保存按逗号切开了登录名");
});

test("含顿号的登录名回填后保存不变", () => {
  assert.deepEqual(roundTripMembers("a、b"), ["a、b", "bob"], "保存按顿号切开了登录名");
});

test("没有列名成员的行回填后直接保存，成员表仍为空", () => {
  const html = renderUsersPage(ownerPage);
  const value = membersValueOf(html, 'data-path="/api/v1/resources/Application/blog?namespace=default"');
  assert.equal(value, "", "没有成员的行把说明文字填进了成员输入框");
  const body = visibilityBody(projectionOf(storedApplication), "blog", value, true);
  assert.deepEqual(body.spec.visibility.members, [], "保存把说明文字写成了成员");
});

test("没有可写行时该项只读", () => {
  const page = {
    ...ownerPage,
    items: ownerPage.items.map((entry) =>
      entry.id === "users-app-visibility" ? { ...entry, value: [] } : entry,
    ),
  };
  const visibility = articleOf(renderUsersPage(page), "users-app-visibility");
  assert.match(visibility, /data-modifiable="false"/, "没有可写行的项被标为可修改");
  assert.ok(visibility.includes("该项当前没有可修改的行"), "没有可写行的项未说明只读");
  assert.ok(!visibility.includes("data-entry"), "没有可写行的项给出了入口");
});

test("可见性写入以存储对象为底稿，请求体带 metadata.name 且保留其余字段", () => {
  const body = visibilityBody(projectionOf(storedApplication), "shop", "bob\ncarol", true);
  assert.equal(body.metadata.name, "shop", "请求体缺 metadata.name");
  assert.equal(body.metadata.namespace, "default", "请求体丢掉了对象命名空间");
  assert.equal(body.metadata.resourceVersion, "42", "请求体丢掉了写入前置条件");
  assert.deepEqual(body.metadata.labels, { "kos.gokos.dev/managed-by": "kos" }, "请求体丢掉了对象标记");
  assert.deepEqual(body.spec.visibility, { members: ["bob", "carol"], allowGuests: true }, "请求体未带新的可见性");
  assert.equal(body.spec.definitionRef, "shop", "请求体丢掉了其余 spec 字段");
  assert.equal(body.spec.owner, "bob", "请求体丢掉了对象所有者");
  assert.equal(body.status, undefined, "请求体带上了由调和器写入的 status");
  assert.equal(body.ref, undefined, "请求体带上了读取信封");
  assert.equal(body.observedAt, undefined, "请求体带上了读取信封");
});

test("所有者转移同样以存储对象为底稿，带 metadata.name 并保留可见性", () => {
  const body = ownerBody(projectionOf(storedApplication), "shop", "alice");
  assert.equal(body.metadata.name, "shop", "请求体缺 metadata.name");
  assert.equal(body.metadata.resourceVersion, "42", "请求体丢掉了写入前置条件");
  assert.equal(body.spec.owner, "alice", "请求体未带新的所有者");
  assert.deepEqual(body.spec.visibility, { members: ["bob", "carol"], allowGuests: false }, "请求体丢掉了可见性");
  assert.equal(body.status, undefined, "请求体带上了由调和器写入的 status");
});

test("数据库不可用时成员与角色给出原因，对象源照常可读", () => {
  const html = renderUsersPage(databaseDownPage);
  const members = articleOf(html, "users-members-and-roles");
  assert.ok(members.includes(`不可用：${DATABASE_REASON}`), "成员与角色未给出数据库不可用的原因");
  assert.equal(countOf(members, "data-entry"), 0, "不可用的成员列表仍带修改入口");
  assert.ok(articleOf(html, "users-app-visibility").includes("default/shop"), "可见性未在数据库不可用时照常读取");
  assert.ok(
    articleOf(html, "users-app-and-volume-owners").includes("所有者：alice"),
    "所有者项未在数据库不可用时照常读取",
  );
});

test("值按文本渲染，不把标记注入页面", () => {
  const page = {
    ...ownerPage,
    items: [{
      ...ownerPage.items[1],
      value: [{ namespace: "default", name: "<script>alert(1)</script>", members: ["<b>bob</b>"], allowGuests: false, writable: true }],
    }],
  };
  const html = renderUsersPage(page);
  assert.ok(!html.includes("<script>"), "页面注入了值里的标记");
  assert.ok(html.includes("&lt;script&gt;"), "路径未按文本转义");
  assert.ok(html.includes("&lt;b&gt;bob&lt;/b&gt;"), "成员名未按文本转义");
});
