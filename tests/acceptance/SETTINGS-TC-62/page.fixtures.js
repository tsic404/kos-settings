// The page projection GET /api/v1/pages/settings answers, trimmed to the
// session's role, as fixtures for the rendered page. The item list, source
// types and reasons are the ones SETTINGS-C-36 declares; the values are the
// shapes the product's read model returns for a two-node cluster.

export const OBSERVED_REASON = "该项由 kos-settings 按集群事实读取，用户不能修改";
export const NODE_CONFIGURATION_REASON =
  "该项是节点主机的配置（如资源预留），不在对象模型内，KOS 不提供读取或修改";

const OBSERVED_AT = "2026-03-01T12:00:00Z";

function item({ id, label, source, value, action = "", reason = OBSERVED_REASON }) {
  return { id, label, source, value, observedAt: OBSERVED_AT, stale: false, action, reason };
}

const clusterItems = [
  item({
    id: "cluster-nodes-and-roles",
    label: "节点与角色",
    source: "object",
    value: [
      { name: "node-a", roles: ["worker"] },
      { name: "node-b", roles: ["control-plane"] },
    ],
  }),
  item({
    id: "cluster-system-components",
    label: "系统组件",
    source: "object",
    value: [{ name: "kosd", phase: "Running", health: "Healthy" }],
  }),
  item({
    id: "cluster-network-and-storage",
    label: "网络与存储能力",
    source: "object",
    value: [
      {
        name: "standard.example",
        version: "1.2.0",
        health: "Healthy",
        capabilities: ["storage.block"],
        storageClasses: ["standard"],
      },
    ],
  }),
  item({
    id: "cluster-version",
    label: "集群版本",
    source: "computed",
    value: {
      versions: ["v1.37.0"],
      nodes: [
        { name: "node-a", version: "v1.37.0" },
        { name: "node-b", version: "v1.37.0" },
      ],
    },
  }),
  item({
    id: "cluster-capabilities",
    label: "已装能力",
    source: "object",
    value: [
      {
        provider: "standard.example",
        capability: "storage.block",
        version: "1",
        scope: "node",
        source: "KOS",
        state: "Verified",
      },
    ],
  }),
  item({
    id: "cluster-health",
    label: "集群健康",
    source: "computed",
    value: { health: "Healthy", reasons: [] },
  }),
  {
    id: "cluster-maintenance-window",
    label: "集群级策略（维护窗口）",
    source: "database",
    value: "Sun 02:00-04:00",
    observedAt: OBSERVED_AT,
    stale: false,
    action: "set-cluster-maintenance-window",
    entry: { method: "PUT", path: "/api/v1/settings/cluster-maintenance-window" },
  },
  item({
    id: "cluster-allocatable",
    label: "可分配容量",
    source: "object",
    value: {
      allocatable: { cpu: "6", memory: "12Gi" },
      nodes: [
        { name: "node-a", allocatable: { cpu: "4", memory: "8Gi" } },
        { name: "node-b", allocatable: { cpu: "2", memory: "4Gi" } },
      ],
    },
  }),
  item({
    id: "cluster-usage",
    label: "实测用量",
    source: "metric",
    value: { available: false, reason: "发行版没有接入资源指标来源，KOS 不采集节点用量" },
  }),
];

const nodeItems = [
  item({
    id: "node-identity",
    label: "身份与标签",
    source: "object",
    value: [
      { name: "node-a", labels: { "node-role.kubernetes.io/worker": "" } },
      { name: "node-b" },
    ],
  }),
  item({
    id: "node-status",
    label: "状态",
    source: "object",
    value: [
      { name: "node-a", ready: true, schedulable: true },
      { name: "node-b", ready: true, schedulable: false },
    ],
  }),
  item({
    id: "node-scheduling-constraints",
    label: "调度约束",
    source: "object",
    value: [
      { name: "node-a", taints: [], schedulable: true },
      { name: "node-b", taints: [], schedulable: false },
    ],
  }),
  item({
    id: "node-devices",
    label: "设备清单",
    source: "object",
    value: [{ name: "node-a", devices: 1, occupied: 0 }],
  }),
  item({
    id: "node-provider-status",
    label: "节点级 Provider 状态",
    source: "object",
    value: [
      { name: "node-a", providers: [{ name: "standard.example", version: "1.2.0", health: "Healthy" }] },
      { name: "node-b", providers: [] },
    ],
  }),
  item({
    id: "node-allocatable",
    label: "可分配容量",
    source: "object",
    value: [
      { name: "node-a", cpu: "4", memory: "8Gi" },
      { name: "node-b", cpu: "2", memory: "4Gi" },
    ],
  }),
  item({
    id: "node-usage",
    label: "实测用量",
    source: "metric",
    value: { available: false, reason: "发行版没有接入资源指标来源，KOS 不采集节点用量" },
  }),
  item({
    id: "node-maintenance-state",
    label: "不可调度状态",
    source: "object",
    value: [
      { name: "node-a", schedulable: true },
      { name: "node-b", schedulable: false },
    ],
  }),
  item({
    id: "node-maintenance-affected",
    label: "维护受影响的应用",
    source: "computed",
    value: [
      { name: "node-a", applications: [] },
      { name: "node-b", applications: ["default/media"] },
    ],
  }),
  item({
    id: "node-maintenance-origin",
    label: "维护归属",
    source: "database",
    value: [
      { name: "node-a", origin: "none" },
      { name: "node-b", origin: "planned" },
    ],
  }),
  item({
    id: "node-level-configuration",
    label: "节点级配置（资源预留等）",
    source: "read",
    value: { available: false, reason: NODE_CONFIGURATION_REASON },
    reason: NODE_CONFIGURATION_REASON,
  }),
];

const OWNER_ACTIONS = [
  {
    id: "set-cluster-maintenance-window",
    entry: { method: "PUT", path: "/api/v1/settings/cluster-maintenance-window" },
  },
  {
    id: "start-node-maintenance",
    entry: { method: "POST", path: "/api/v1/node-maintenance?node={node}&downtime={downtime}" },
  },
  {
    id: "restore-node-maintenance",
    entry: { method: "DELETE", path: "/api/v1/node-maintenance/{node}" },
  },
];

// ownerPage is the projection an Owner reads: every action and its entry.
export const ownerPage = {
  id: "settings",
  title: "系统设置",
  location: "panel",
  requiredRole: "",
  readOnly: false,
  readOnlyReason: "",
  sources: ["object", "computed", "database", "metric", "read"],
  items: [...clusterItems, ...nodeItems],
  actions: OWNER_ACTIONS,
};

// memberPage is the same page trimmed to a Member: the server omits every
// action and every entry, so the page is read-only without inventing a reason
// for the role restriction (KOSD-C-59).
export const memberPage = {
  ...ownerPage,
  items: ownerPage.items.map((entry) =>
    entry.entry
      ? { ...entry, entry: undefined }
      : entry,
  ),
  actions: [],
};

export const pageItems = ownerPage.items;
