// Package kossettings is the kos-settings product: the cluster and node
// settings the panel shows (C-36) and the modifiable item the product
// contributes (C-23), with the sources its settings page reads (C-53). Every
// item below is this package's own declaration; the kernel hardcodes none of
// them, so the items appear and disappear with the component (C-23, F16).
package kossettings

import (
	"net/http"

	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/write"
)

// Identifier is the product's extension identity across versions (C-17).
const Identifier = "kos-settings"

// Version is the packaged product version.
const Version = "1.0.0"

// FrameworkVersion is the framework range this package targets; the kernel
// serves 1.1, and a declaration that admits none of the served versions is
// incompatible (C-16).
const FrameworkVersion = ">=1.0 <2.0"

// Item identities of the cluster settings C-36 covers, one item per declared
// source. Resource capacity and sampled consumption stay separate items
// because each is judged by its own source type (C-36, C-53, C-71).
const (
	ClusterNodesAndRoles     = "cluster-nodes-and-roles"
	ClusterSystemComponents  = "cluster-system-components"
	ClusterNetworkAndStorage = "cluster-network-and-storage"
	ClusterVersion           = "cluster-version"
	ClusterCapabilities      = "cluster-capabilities"
	ClusterHealth            = "cluster-health"
	ClusterMaintenanceWindow = "cluster-maintenance-window"
	ClusterAllocatable       = "cluster-allocatable"
	ClusterUsage             = "cluster-usage"
)

// Item identities of the node settings C-36 covers.
const (
	NodeIdentity              = "node-identity"
	NodeStatus                = "node-status"
	NodeSchedulingConstraints = "node-scheduling-constraints"
	NodeDevices               = "node-devices"
	NodeProviderStatus        = "node-provider-status"
	NodeAllocatable           = "node-allocatable"
	NodeUsage                 = "node-usage"
	NodeMaintenanceState      = "node-maintenance-state"
	NodeMaintenanceAffected   = "node-maintenance-affected-apps"
	NodeMaintenanceOrigin     = "node-maintenance-origin"
	NodeLevelConfiguration    = "node-level-configuration"
)

// PageSettings is the settings page this product declares (C-22, C-36).
const PageSettings = "settings"

// Action identities of the settings page. A node-scoped write needs the node it
// applies to, so its path carries the "{node}" placeholder the browser fills
// with the node chosen in the panel. A maintenance start also needs the declared
// downtime estimate, which is what makes the preflight judgeable (C-55, C-65).
const (
	ActionSetMaintenanceWindow = "set-cluster-maintenance-window"
	ActionStartMaintenance     = "start-node-maintenance"
	ActionRestoreMaintenance   = "restore-node-maintenance"
)

// ObservedReason is the reason a fact item carries: the product reads it from
// the cluster or the control-plane database and the user does not write it
// (C-36, C-53).
const ObservedReason = "该项由 kos-settings 按集群事实读取，用户不能修改"

// NodeConfigurationReason explains the node-level configuration that lives
// outside the object model, such as resource reservations (C-36).
const NodeConfigurationReason = "该项是节点主机的配置（如资源预留），不在对象模型内，KOS 不提供读取或修改"

// Declaration returns the contribution kos-settings ships (C-18): the one
// setting item the user owns and the settings page C-36 puts in the panel.
func Declaration() extension.Declaration {
	return extension.Declaration{
		Identifier:       Identifier,
		Version:          Version,
		FrameworkVersion: FrameworkVersion,
		Contributions: extension.Contributions{
			Settings: []extension.Setting{{
				// A cluster-level policy the user owns (C-36, C-72).
				ID:       ClusterMaintenanceWindow,
				Scope:    extension.SettingScopeCluster,
				Writer:   extension.SettingWriterUser,
				Editable: true,
			}},
			Pages: []extension.Page{settingsPage()},
		},
	}
}

// reference is the item reference this product owns; the panel shell dispatches
// an item to the reader of the product in front of it (C-22).
func reference(item string) string { return Identifier + "/" + item }

// settingsPage declares every cluster and node setting C-36 covers. A fact is
// read from its source and carries a reason instead of an entry; the modifiable
// policy item references the control-plane write that changes it.
func settingsPage() extension.Page {
	return extension.Page{
		ID:       PageSettings,
		Title:    "系统设置",
		Location: "panel",
		Items: []extension.PageItem{
			// Cluster group (C-36).
			{ID: ClusterNodesAndRoles, Label: "节点与角色", Source: extension.PageSourceObject, Reference: reference(ClusterNodesAndRoles), Reason: ObservedReason},
			{ID: ClusterSystemComponents, Label: "系统组件", Source: extension.PageSourceObject, Reference: reference(ClusterSystemComponents), Reason: ObservedReason},
			{ID: ClusterNetworkAndStorage, Label: "网络与存储能力", Source: extension.PageSourceObject, Reference: reference(ClusterNetworkAndStorage), Reason: ObservedReason},
			{ID: ClusterVersion, Label: "集群版本", Source: extension.PageSourceComputed, Reference: reference(ClusterVersion), Reason: ObservedReason},
			{ID: ClusterCapabilities, Label: "已装能力", Source: extension.PageSourceObject, Reference: reference(ClusterCapabilities), Reason: ObservedReason},
			{ID: ClusterHealth, Label: "集群健康", Source: extension.PageSourceComputed, Reference: reference(ClusterHealth), Reason: ObservedReason},
			{ID: ClusterMaintenanceWindow, Label: "集群级策略（维护窗口）", Source: extension.PageSourceDatabase, Reference: reference(ClusterMaintenanceWindow), Action: ActionSetMaintenanceWindow},
			{ID: ClusterAllocatable, Label: "可分配容量", Source: extension.PageSourceObject, Reference: reference(ClusterAllocatable), Reason: ObservedReason},
			{ID: ClusterUsage, Label: "实测用量", Source: extension.PageSourceMetric, Reference: reference(ClusterUsage), Reason: ObservedReason},
			// Node group (C-36). Each value is keyed by node, because the panel
			// reads one node at a time from the same projection.
			{ID: NodeIdentity, Label: "身份与标签", Source: extension.PageSourceObject, Reference: reference(NodeIdentity), Reason: ObservedReason},
			{ID: NodeStatus, Label: "状态", Source: extension.PageSourceObject, Reference: reference(NodeStatus), Reason: ObservedReason},
			{ID: NodeSchedulingConstraints, Label: "调度约束", Source: extension.PageSourceObject, Reference: reference(NodeSchedulingConstraints), Reason: ObservedReason},
			{ID: NodeDevices, Label: "设备清单", Source: extension.PageSourceObject, Reference: reference(NodeDevices), Reason: ObservedReason},
			{ID: NodeProviderStatus, Label: "节点级 Provider 状态", Source: extension.PageSourceObject, Reference: reference(NodeProviderStatus), Reason: ObservedReason},
			{ID: NodeAllocatable, Label: "可分配容量", Source: extension.PageSourceObject, Reference: reference(NodeAllocatable), Reason: ObservedReason},
			{ID: NodeUsage, Label: "实测用量", Source: extension.PageSourceMetric, Reference: reference(NodeUsage), Reason: ObservedReason},
			{ID: NodeMaintenanceState, Label: "不可调度状态", Source: extension.PageSourceObject, Reference: reference(NodeMaintenanceState), Reason: ObservedReason},
			{ID: NodeMaintenanceAffected, Label: "维护受影响的应用", Source: extension.PageSourceComputed, Reference: reference(NodeMaintenanceAffected), Reason: ObservedReason},
			{ID: NodeMaintenanceOrigin, Label: "维护归属", Source: extension.PageSourceDatabase, Reference: reference(NodeMaintenanceOrigin), Reason: ObservedReason},
			{ID: NodeLevelConfiguration, Label: "节点级配置（资源预留等）", Source: extension.PageSourceRead, Reference: reference(NodeLevelConfiguration), Reason: NodeConfigurationReason},
		},
		Actions: []extension.Action{
			// The cluster policy is the Owner's write (C-59, C-72).
			{ID: ActionSetMaintenanceWindow, Method: http.MethodPut, Path: "/api/v1/settings/" + ClusterMaintenanceWindow, RequiredRole: string(write.RoleOwner)},
			// Node maintenance is the Owner's action alone (C-59, C-65): the
			// entries appear for that role and for no other.
			{ID: ActionStartMaintenance, Method: http.MethodPost, Path: "/api/v1/node-maintenance?node={node}&downtime={downtime}", RequiredRole: string(write.RoleOwner)},
			{ID: ActionRestoreMaintenance, Method: http.MethodDelete, Path: "/api/v1/node-maintenance/{node}", RequiredRole: string(write.RoleOwner)},
		},
	}
}
