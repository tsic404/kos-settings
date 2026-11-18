// The page projection GET /api/v1/pages/settings answers for the two
// resource-occupancy groups, one page per state SETTINGS-TC-135 drives. The
// capacity values are the node objects' allocatable fields and do not move
// between states; the usage values are the metric source's projection, which
// either reports every node's sample, an empty window, one node short, or the
// absence of a source (SETTINGS-C-71).

const OBSERVED_AT = "2026-03-01T12:00:00Z";
const OBSERVED_REASON = "该项由 kos-settings 按集群事实读取，用户不能修改";

// The two-node cluster the case reads: capacity is readable in all four states.
export const CLUSTER_CAPACITY = { cpu: "6", memory: "12Gi" };
export const NODE_CAPACITY = [
  { name: "node-a", cpu: "4", memory: "8Gi" },
  { name: "node-b", cpu: "2", memory: "4Gi" },
];

// The samples the full window reports, and their sum over both nodes.
export const SAMPLES = [
  { name: "node-a", cpu: "1", memory: "1Gi" },
  { name: "node-b", cpu: "2", memory: "2Gi" },
];
export const SAMPLE_TOTAL = { cpu: "3", memory: "3Gi" };

export const REASONS = {
  empty: "资源指标缺少节点样本：node-a, node-b",
  missingNode: "资源指标缺少节点样本：node-b",
  detached: "发行版没有接入资源指标来源，KOS 不采集节点用量",
};

function item({ id, label, source, value }) {
  return { id, label, source, value, observedAt: OBSERVED_AT, stale: false, action: "", reason: OBSERVED_REASON };
}

// pageOf renders one state's projection: capacity always carries the node
// objects' allocatable values, usage carries what the metric source answered.
export function pageOf({ clusterUsage, nodeUsage }) {
  return {
    id: "settings",
    title: "系统设置",
    location: "panel",
    requiredRole: "",
    readOnly: true,
    readOnlyReason: "",
    sources: ["object", "metric"],
    items: [
      item({
        id: "cluster-allocatable",
        label: "可分配容量",
        source: "object",
        value: {
          allocatable: { ...CLUSTER_CAPACITY },
          nodes: NODE_CAPACITY.map((node) => ({
            name: node.name,
            allocatable: { cpu: node.cpu, memory: node.memory },
          })),
        },
      }),
      item({ id: "cluster-usage", label: "实测用量", source: "metric", value: clusterUsage }),
      item({
        id: "node-allocatable",
        label: "可分配容量",
        source: "object",
        value: NODE_CAPACITY.map((node) => ({ ...node })),
      }),
      item({ id: "node-usage", label: "实测用量", source: "metric", value: nodeUsage }),
    ],
    actions: [],
  };
}

// full covers both nodes, so both levels report the samples and the cluster
// total is their sum (C-71).
export const fullPage = pageOf({
  clusterUsage: {
    available: true,
    used: { ...SAMPLE_TOTAL },
    nodes: SAMPLES.map((sample) => ({ name: sample.name, used: { cpu: sample.cpu, memory: sample.memory } })),
  },
  nodeUsage: { available: true, nodes: SAMPLES.map((sample) => ({ ...sample })) },
});

// emptyPage is the window that sampled no node, oneNodeShort names the node the
// source left out, and detachedPage is the distribution that integrates no
// metric source (C-71).
export const emptyPage = pageOf({
  clusterUsage: { available: false, reason: REASONS.empty },
  nodeUsage: { available: false, reason: REASONS.empty },
});
export const oneNodeShortPage = pageOf({
  clusterUsage: { available: false, reason: REASONS.missingNode },
  nodeUsage: { available: false, reason: REASONS.missingNode },
});
export const detachedPage = pageOf({
  clusterUsage: { available: false, reason: REASONS.detached },
  nodeUsage: { available: false, reason: REASONS.detached },
});
