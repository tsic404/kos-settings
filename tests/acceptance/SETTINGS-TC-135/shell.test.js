// The rendered resource-occupancy groups under the four states SETTINGS-TC-135
// drives: capacity stays readable in every state, a full window renders both
// levels' usage, and an empty window, a node short or no metric source renders
// the item unavailable with its reason and no usage at any level
// (SETTINGS-AC-64, SETTINGS-TC-135, SETTINGS-C-71).

import test from "node:test";
import assert from "node:assert/strict";

import { formatValue, renderSettingsPage } from "../../../assets/settings/render.js";
import {
  CLUSTER_CAPACITY,
  NODE_CAPACITY,
  REASONS,
  SAMPLES,
  SAMPLE_TOTAL,
  detachedPage,
  emptyPage,
  fullPage,
  oneNodeShortPage,
} from "./page.fixtures.js";

const UNAVAILABLE = "不可用：";
const CLUSTER_CAPACITY_ITEM = "cluster-allocatable";
const CLUSTER_USAGE_ITEM = "cluster-usage";
const NODE_CAPACITY_ITEM = "node-allocatable";
const NODE_USAGE_ITEM = "node-usage";

// articleOf returns one item's rendered article.
function articleOf(html, id) {
  const marker = `data-item="${id}"`;
  const start = html.indexOf(marker);
  if (start < 0) return "";
  const open = html.lastIndexOf("<article", start);
  const close = html.indexOf("</article>", start);
  return html.slice(open, close < 0 ? undefined : close);
}

// valueOf returns one item's rendered value text.
function valueOf(html, id) {
  const matched = articleOf(html, id).match(/<p class="item-value" data-value>([\s\S]*?)<\/p>/);
  return matched ? matched[1].trim() : "";
}

// valuesByItem renders one page and returns every item's value text.
function valuesByItem(page) {
  const html = renderSettingsPage(page);
  return {
    html,
    clusterCapacity: valueOf(html, CLUSTER_CAPACITY_ITEM),
    clusterUsage: valueOf(html, CLUSTER_USAGE_ITEM),
    nodeCapacity: valueOf(html, NODE_CAPACITY_ITEM),
    nodeUsage: valueOf(html, NODE_USAGE_ITEM),
  };
}

// assertCapacity checks that both capacity items render the nodes' allocatable
// values and are not marked unavailable (C-71).
function assertCapacity(rendered, state) {
  assert.ok(
    rendered.clusterCapacity.includes(`cpu=${CLUSTER_CAPACITY.cpu}`),
    `${state} 的集群容量未渲染 CPU：${rendered.clusterCapacity}`,
  );
  assert.ok(
    rendered.clusterCapacity.includes(`memory=${CLUSTER_CAPACITY.memory}`),
    `${state} 的集群容量未渲染内存：${rendered.clusterCapacity}`,
  );
  for (const node of NODE_CAPACITY) {
    assert.ok(rendered.nodeCapacity.includes(node.name), `${state} 的节点容量未点名 ${node.name}`);
    assert.ok(
      rendered.nodeCapacity.includes(`${node.name}：cpu=${node.cpu}；memory=${node.memory}`),
      `${state} 的 ${node.name} 容量未渲染为 ${node.cpu}/${node.memory}：${rendered.nodeCapacity}`,
    );
  }
  for (const [group, text] of [
    ["集群", rendered.clusterCapacity],
    ["节点", rendered.nodeCapacity],
  ]) {
    assert.equal(text.includes(UNAVAILABLE), false, `${state} 的${group}容量被标为不可用：${text}`);
  }
}

// assertUnavailable checks that one usage item renders the given reason and
// carries no usage value at any level (C-71).
function assertUnavailable(rendered, key, item, reason, state) {
  assert.ok(rendered[key].startsWith(UNAVAILABLE), `${state} 的 ${item} 未标为不可用：${rendered[key]}`);
  assert.ok(rendered[key].includes(reason), `${state} 的 ${item} 未给出原因：${rendered[key]}`);
  for (const field of ["used", "nodes", "cpu", "memory"]) {
    assert.equal(rendered[key].includes(`${field}=`), false, `${state} 的 ${item} 带出了用量字段 ${field}`);
  }
}

// assertUsageRendered checks that both usage levels render the samples the
// metric source reported, with the cluster total over every node (C-71).
function assertUsageRendered(rendered) {
  assert.equal(rendered.clusterUsage.includes(UNAVAILABLE), false, `完整样本的集群用量被判为不可用：${rendered.clusterUsage}`);
  assert.equal(rendered.nodeUsage.includes(UNAVAILABLE), false, `完整样本的节点用量被判为不可用：${rendered.nodeUsage}`);

  for (const [level, text] of [
    ["集群", rendered.clusterUsage],
    ["节点", rendered.nodeUsage],
  ]) {
    for (const sample of SAMPLES) {
      assert.ok(text.includes(sample.name), `完整样本的${level}用量未点名 ${sample.name}`);
      assert.ok(text.includes(`cpu=${sample.cpu}`), `完整样本的${level}用量未渲染 ${sample.name} 的 CPU`);
      assert.ok(text.includes(`memory=${sample.memory}`), `完整样本的${level}用量未渲染 ${sample.name} 的内存`);
    }
  }

  assert.ok(
    rendered.clusterUsage.includes(`cpu=${SAMPLE_TOTAL.cpu}`) &&
      rendered.clusterUsage.includes(`memory=${SAMPLE_TOTAL.memory}`),
    `完整样本的集群用量未渲染各节点之和 ${SAMPLE_TOTAL.cpu}/${SAMPLE_TOTAL.memory}：${rendered.clusterUsage}`,
  );
}

test("四态下集群与节点的可分配容量都可读", () => {
  for (const [state, page] of [
    ["完整样本", fullPage],
    ["空样本集", emptyPage],
    ["缺一个节点样本", oneNodeShortPage],
    ["未接入指标源", detachedPage],
  ]) {
    assertCapacity(valuesByItem(page), state);
  }
});

test("完整样本时集群与各节点用量渲染出样本，且不标为不可用", () => {
  assertUsageRendered(valuesByItem(fullPage));
});

test("空样本集时两级用量不可用，原因点名未覆盖的每个节点", () => {
  const rendered = valuesByItem(emptyPage);
  assertUnavailable(rendered, "clusterUsage", CLUSTER_USAGE_ITEM, REASONS.empty, "空样本集");
  assertUnavailable(rendered, "nodeUsage", NODE_USAGE_ITEM, REASONS.empty, "空样本集");
  for (const node of NODE_CAPACITY) {
    assert.ok(rendered.clusterUsage.includes(node.name), `空样本集的原因未点名 ${node.name}`);
  }
});

test("缺一个节点样本时两级用量不可用，原因点名未覆盖的节点", () => {
  const rendered = valuesByItem(oneNodeShortPage);
  assertUnavailable(rendered, "clusterUsage", CLUSTER_USAGE_ITEM, REASONS.missingNode, "缺一个节点样本");
  assertUnavailable(rendered, "nodeUsage", NODE_USAGE_ITEM, REASONS.missingNode, "缺一个节点样本");
  assert.ok(rendered.clusterUsage.includes("node-b"), "缺样本的原因未点名未覆盖的 node-b");
  assert.equal(rendered.clusterUsage.includes("node-a"), false, "缺样本的原因点名了已覆盖的 node-a");

  // The one sample the window carries is not the cluster's usage.
  for (const sample of SAMPLES) {
    assert.equal(
      rendered.clusterUsage.includes(`${sample.name}：`),
      false,
      `缺样本时集群项仍渲染了 ${sample.name} 的样本`,
    );
  }
});

test("未接入指标源时两级用量不可用并给出原因", () => {
  const rendered = valuesByItem(detachedPage);
  assertUnavailable(rendered, "clusterUsage", CLUSTER_USAGE_ITEM, REASONS.detached, "未接入指标源");
  assertUnavailable(rendered, "nodeUsage", NODE_USAGE_ITEM, REASONS.detached, "未接入指标源");
});

test("不可用的用量项不把容量呈现为用量", () => {
  for (const [state, page] of [
    ["空样本集", emptyPage],
    ["缺一个节点样本", oneNodeShortPage],
    ["未接入指标源", detachedPage],
  ]) {
    const rendered = valuesByItem(page);
    for (const [level, text] of [
      [CLUSTER_USAGE_ITEM, rendered.clusterUsage],
      [NODE_USAGE_ITEM, rendered.nodeUsage],
    ]) {
      for (const capacity of [CLUSTER_CAPACITY, ...NODE_CAPACITY]) {
        assert.equal(
          text.includes(`cpu=${capacity.cpu}`) && text.includes(`memory=${capacity.memory}`),
          false,
          `${state} 的 ${level} 把容量 ${capacity.cpu}/${capacity.memory} 呈现为用量：${text}`,
        );
      }
      assert.equal(
        text.includes(`allocatable=`),
        false,
        `${state} 的 ${level} 带出了容量字段：${text}`,
      );
    }
  }
});

test("formatValue 对不可用投影只给出原因，对可用投影给出键值", () => {
  assert.equal(
    formatValue({ available: false, reason: REASONS.detached }),
    `${UNAVAILABLE}${REASONS.detached}`,
  );
  const rendered = formatValue(fullPage.items.find((entry) => entry.id === CLUSTER_USAGE_ITEM).value);
  assert.equal(rendered.includes(UNAVAILABLE), false, "可用用量被渲染为不可用");
  assert.ok(
    rendered.includes(`${SAMPLES[0].name}；used=cpu=${SAMPLES[0].cpu}；memory=${SAMPLES[0].memory}`),
    "可用用量未按投影形状渲染节点样本",
  );
});
