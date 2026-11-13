package kossettings

import (
	"context"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/pages/cluster"
	"github.com/tsic404/kosd/pkg/settings"
)

// maintenanceVerbs is the pair of records C-65 judges a node's origin by: the
// write that cordons it and the write that lifts the cordon. The maintenance
// flow and this reader share the vocabulary, and the read asks for this pair
// alone, so the node's own maintenance history is the window.
var maintenanceVerbs = []string{controlplane.VerbMaintain, controlplane.VerbRestore}

// maintenanceResults are the outcomes that decide an origin: only a write that
// took effect moved the unschedulable field. A refused or failed attempt keeps
// its record (C-14) but leaves the field alone, so it must not flip a node
// between planned and unplanned maintenance (C-65).
var maintenanceResults = []string{controlplane.ResultAccepted}

// Maintenance origin values C-36 reports.
const (
	MaintenanceNone      = "none"
	MaintenancePlanned   = "planned"
	MaintenanceUnplanned = "unplanned"
	MaintenanceUnknown   = "unknown"
)

// UsageSource reports the sampled CPU and memory consumption of the nodes
// (C-36, C-49). A distribution with no metric source leaves it nil, and the
// usage item then states that instead of presenting capacity as consumption.
type UsageSource interface {
	// NodeUsage returns one sample per node.
	NodeUsage(ctx context.Context) ([]NodeUsageSample, error)
}

// NodeUsageSample is one node's sampled consumption.
type NodeUsageSample struct {
	// Name is the node the sample belongs to.
	Name string `json:"name"`
	// CPU is the sampled CPU usage as a quantity.
	CPU string `json:"cpu"`
	// Memory is the sampled memory usage as a quantity.
	Memory string `json:"memory"`
}

// SettingReader projects the setting items the product declared (C-23).
type SettingReader interface {
	Items(ctx context.Context, session identity.Session, node string) ([]settings.Item, *controlplane.Error)
}

// AuditReader reads the control-plane audit records the maintenance origin
// comes from (C-14, C-65). A read is scoped to one node's own records and to
// the outcomes that took effect, so neither the size of the rest of the trail
// nor a refused attempt decides the answer.
type AuditReader interface {
	RecentAuditEntriesForObject(ctx context.Context, object string, verbs, results []string, limit int) ([]identity.AuditEntry, error)
}

// Readings resolves every item the product's pages declare (C-53): the cluster
// and node facts of C-36, the setting values it contributes and the user page's
// member list and ownership facts (C-1). Each read stamps the moment it observed
// the value, so the page shell applies the one freshness bound of C-13.
type Readings struct {
	// Dynamic reads the cluster objects the object and computed items judge.
	Dynamic dynamic.Interface
	// Settings projects the setting items this product contributes.
	Settings SettingReader
	// Audit reads the maintenance trail; nil leaves the origin unknown.
	Audit AuditReader
	// Usage reports the sampled node consumption of C-36's usage items; nil
	// leaves the item stating that no metric source is configured.
	Usage UsageSource
	// Members reads the accounts the user page shows (C-1); nil or a failing
	// read leaves the item stating why it cannot list them (KOSD-C-61).
	Members MemberSource
	// Now supplies the read clock; defaults to time.Now.
	Now func() time.Time
}

// Read implements pages.Readings.
func (r *Readings) Read(ctx context.Context, session identity.Session, item extension.PageItem) (pages.Reading, *controlplane.Error) {
	value, failure := r.value(ctx, session, item)
	if failure != nil {
		return pages.Reading{}, failure
	}
	return pages.Reading{Value: value, ObservedAt: r.now()}, nil
}

// value resolves one item from the source it declares.
func (r *Readings) value(ctx context.Context, session identity.Session, item extension.PageItem) (any, *controlplane.Error) {
	switch item.ID {
	case ClusterNodesAndRoles:
		return r.clusterNodes(ctx)
	case ClusterSystemComponents:
		return r.systemComponents(ctx)
	case ClusterNetworkAndStorage:
		return r.providers(ctx)
	case ClusterVersion:
		return r.clusterVersions(ctx)
	case ClusterCapabilities:
		return r.capabilities(ctx)
	case ClusterHealth:
		return r.clusterHealth(ctx)
	case ClusterMaintenanceWindow:
		return r.settingValue(ctx, session, ClusterMaintenanceWindow)
	case ClusterAllocatable:
		return r.clusterAllocatable(ctx)
	case ClusterUsage:
		return r.clusterUsage(ctx)
	case NodeIdentity:
		return r.nodeIdentities(ctx)
	case NodeStatus:
		return r.nodeStatuses(ctx)
	case NodeSchedulingConstraints:
		return r.nodeConstraints(ctx)
	case NodeDevices:
		return r.nodeDevices(ctx)
	case NodeProviderStatus:
		return r.nodeProviders(ctx)
	case NodeAllocatable:
		return r.nodeAllocatable(ctx)
	case NodeUsage:
		return r.nodeUsageItem(ctx)
	case NodeMaintenanceState:
		return r.nodeMaintenanceStates(ctx)
	case NodeMaintenanceAffected:
		return r.nodeMaintenanceAffected(ctx)
	case NodeMaintenanceOrigin:
		return r.nodeMaintenanceOrigins(ctx)
	case NodeLevelConfiguration:
		// The configuration lives on the host, outside the object model; the
		// item states that instead of showing a made-up value (C-36).
		return unavailableValue(NodeConfigurationReason), nil
	case UserMembersAndRoles:
		return r.memberRoles(ctx)
	case UserAppVisibility:
		return r.appVisibility(ctx, session)
	case UserAppAndVolumeOwners:
		return r.appAndVolumeOwners(ctx, session)
	default:
		return nil, unreadable("页面项 " + item.ID + " 没有来源")
	}
}

// nodeRoles is one node and the roles its labels declare.
type nodeRoles struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// clusterNodes reports the cluster's nodes with the roles their labels declare
// (C-36).
func (r *Readings) clusterNodes(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeRoles, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, nodeRoles{Name: node.Name, Roles: rolesOf(node)})
	}
	return entries, nil
}

// nodeVersion is one node's kubelet version.
type nodeVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// clusterVersions reports the kubelet version every node runs; the set is the
// cluster's versions, and a mixed set stays visible (C-36, UPDATER-C-1).
func (r *Readings) clusterVersions(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	seen := map[string]bool{}
	versions := make([]string, 0, len(nodes))
	perNode := make([]nodeVersion, 0, len(nodes))
	for _, node := range nodes {
		version := node.Status.NodeInfo.KubeletVersion
		perNode = append(perNode, nodeVersion{Name: node.Name, Version: version})
		if version == "" || seen[version] {
			continue
		}
		seen[version] = true
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return map[string]any{"versions": versions, "nodes": perNode}, nil
}

// clusterAllocatable reports what the nodes offer together and per node
// (C-36, C-71). It reads the node objects only, so a missing metric source
// leaves it readable.
func (r *Readings) clusterAllocatable(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	type nodeCapacity struct {
		Name        string            `json:"name"`
		Allocatable map[string]string `json:"allocatable"`
	}
	capacityCPU := apiresource.Quantity{}
	capacityMemory := apiresource.Quantity{}
	entries := make([]nodeCapacity, 0, len(nodes))
	for _, node := range nodes {
		cpu := allocatable(node, corev1.ResourceCPU)
		memory := allocatable(node, corev1.ResourceMemory)
		capacityCPU.Add(cpu)
		capacityMemory.Add(memory)
		entries = append(entries, nodeCapacity{
			Name:        node.Name,
			Allocatable: map[string]string{"cpu": cpu.String(), "memory": memory.String()},
		})
	}
	return map[string]any{
		"allocatable": map[string]string{"cpu": capacityCPU.String(), "memory": capacityMemory.String()},
		"nodes":       entries,
	}, nil
}

// clusterUsage reports the sampled consumption of the cluster and of every
// node (C-36, C-71). Consumption is published only when the metric source
// covers every node: a partial sum is not the cluster's usage, and capacity is
// never presented as consumption.
func (r *Readings) clusterUsage(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	usage, reason := r.samples(ctx, nodes)
	if reason != "" {
		return map[string]any{"available": false, "reason": reason}, nil
	}
	type nodeConsumption struct {
		Name string            `json:"name"`
		Used map[string]string `json:"used"`
	}
	usedCPU := apiresource.Quantity{}
	usedMemory := apiresource.Quantity{}
	entries := make([]nodeConsumption, 0, len(nodes))
	for _, node := range nodes {
		sample := usage[node.Name]
		usedCPU.Add(parseQuantity(sample.CPU))
		usedMemory.Add(parseQuantity(sample.Memory))
		entries = append(entries, nodeConsumption{
			Name: node.Name,
			Used: map[string]string{"cpu": sample.CPU, "memory": sample.Memory},
		})
	}
	return map[string]any{
		"available": true,
		"used":      map[string]string{"cpu": usedCPU.String(), "memory": usedMemory.String()},
		"nodes":     entries,
	}, nil
}

// nodeUsageItem reports the sampled consumption of each node (C-36, C-71).
// When the source is absent, fails or leaves a node unsampled, the item states
// that and carries no usage at any level.
func (r *Readings) nodeUsageItem(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	usage, reason := r.samples(ctx, nodes)
	if reason != "" {
		return map[string]any{"available": false, "reason": reason}, nil
	}
	type nodeConsumption struct {
		Name string `json:"name"`
		CPU  string `json:"cpu"`
		Mem  string `json:"memory"`
	}
	entries := make([]nodeConsumption, 0, len(nodes))
	for _, node := range nodes {
		sample := usage[node.Name]
		entries = append(entries, nodeConsumption{Name: node.Name, CPU: sample.CPU, Mem: sample.Memory})
	}
	return map[string]any{"available": true, "nodes": entries}, nil
}

// samples reads one sample per node and judges coverage. The second result
// explains why usage is unavailable and is empty only when every node was
// sampled; it names the nodes the source left out (C-71).
func (r *Readings) samples(ctx context.Context, nodes []corev1.Node) (map[string]NodeUsageSample, string) {
	if r.Usage == nil {
		return nil, "发行版没有接入资源指标来源，KOS 不采集节点用量"
	}
	samples, err := r.Usage.NodeUsage(ctx)
	if err != nil {
		return nil, "资源指标读取失败，节点用量暂不可读"
	}
	byNode := make(map[string]NodeUsageSample, len(samples))
	for _, sample := range samples {
		if sample.Name == "" {
			continue
		}
		if _, err := apiresource.ParseQuantity(sample.CPU); err != nil {
			return nil, "资源指标样本无法解析，节点用量暂不可读"
		}
		if _, err := apiresource.ParseQuantity(sample.Memory); err != nil {
			return nil, "资源指标样本无法解析，节点用量暂不可读"
		}
		byNode[sample.Name] = sample
	}
	missing := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if _, sampled := byNode[node.Name]; !sampled {
			missing = append(missing, node.Name)
		}
	}
	if len(missing) > 0 {
		return nil, "资源指标缺少节点样本：" + strings.Join(missing, ", ")
	}
	return byNode, ""
}

// parseQuantity reads one sampled quantity the caller already validated.
func parseQuantity(value string) apiresource.Quantity {
	quantity, _ := apiresource.ParseQuantity(value)
	return quantity
}

// componentSummary is one system component as the panel reads it.
type componentSummary struct {
	Name   string `json:"name"`
	Phase  string `json:"phase,omitempty"`
	Health string `json:"health,omitempty"`
}

// systemComponents reports the installed system components and their health
// (C-36).
func (r *Readings) systemComponents(ctx context.Context) (any, *controlplane.Error) {
	objects, failure := r.kernel(ctx, "SystemComponent")
	if failure != nil {
		return nil, failure
	}
	entries := make([]componentSummary, 0, len(objects))
	for _, object := range objects {
		var component v1beta1.SystemComponent
		if failure := decode(object, &component); failure != nil {
			return nil, failure
		}
		entries = append(entries, componentSummary{
			Name:   component.Name,
			Phase:  component.Status.Phase,
			Health: string(component.Status.Health.State),
		})
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name < entries[right].Name })
	return entries, nil
}

// providerSummary is one registered Provider as the panel reads it.
type providerSummary struct {
	Name           string   `json:"name"`
	Version        string   `json:"version,omitempty"`
	Health         string   `json:"health,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	StorageClasses []string `json:"storageClasses,omitempty"`
}

// providers reports the registered providers with their capabilities and
// storage classes; it is the network and storage part of C-36.
func (r *Readings) providers(ctx context.Context) (any, *controlplane.Error) {
	objects, failure := r.kernel(ctx, "Provider")
	if failure != nil {
		return nil, failure
	}
	entries := make([]providerSummary, 0, len(objects))
	for _, object := range objects {
		var provider v1beta1.Provider
		if failure := decode(object, &provider); failure != nil {
			return nil, failure
		}
		entries = append(entries, summaryOf(provider))
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name < entries[right].Name })
	return entries, nil
}

// capabilitySummary is one installed capability with the verification result
// the provider status carries (C-36, UPDATER-C-1).
type capabilitySummary struct {
	Provider   string `json:"provider"`
	Capability string `json:"capability"`
	Version    string `json:"version"`
	Scope      string `json:"scope,omitempty"`
	Source     string `json:"source,omitempty"`
	State      string `json:"state,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// capabilities reports the capabilities registered by the providers together
// with the verification results: registration is object state, and a
// self-report does not replace a KOS verification (C-09, C-10).
func (r *Readings) capabilities(ctx context.Context) (any, *controlplane.Error) {
	objects, failure := r.kernel(ctx, "Provider")
	if failure != nil {
		return nil, failure
	}
	entries := []capabilitySummary{}
	for _, object := range objects {
		var provider v1beta1.Provider
		if failure := decode(object, &provider); failure != nil {
			return nil, failure
		}
		results := map[string]v1beta1.ProviderCapabilityResult{}
		for _, result := range provider.Status.Capabilities {
			results[result.Name+"@"+result.Version] = result
		}
		for _, capability := range provider.Spec.Capabilities {
			entry := capabilitySummary{
				Provider:   provider.Name,
				Capability: capability.Name,
				Version:    capability.Version,
				Scope:      string(capability.Scope),
			}
			if result, verified := results[capability.Name+"@"+capability.Version]; verified {
				entry.Source = result.Source
				entry.State = result.State
				entry.Reason = result.Reason
			}
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Provider != entries[right].Provider {
			return entries[left].Provider < entries[right].Provider
		}
		if entries[left].Capability != entries[right].Capability {
			return entries[left].Capability < entries[right].Capability
		}
		return entries[left].Version < entries[right].Version
	})
	return entries, nil
}

// clusterHealth aggregates the facts C-36 lists as cluster health: every node
// ready and every system component healthy (C-53). A failing fact is named, so
// a degraded cluster says what degraded.
func (r *Readings) clusterHealth(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	reasons := []string{}
	for _, node := range nodes {
		if !isReady(node) {
			reasons = append(reasons, "节点 "+node.Name+" 未就绪")
		}
	}
	objects, failure := r.kernel(ctx, "SystemComponent")
	if failure != nil {
		return nil, failure
	}
	for _, object := range objects {
		var component v1beta1.SystemComponent
		if failure := decode(object, &component); failure != nil {
			return nil, failure
		}
		if component.Status.Health.State != v1beta1.HealthStateHealthy {
			reasons = append(reasons, "系统组件 "+component.Name+" 健康为 "+string(component.Status.Health.State))
		}
	}
	health := string(v1beta1.HealthStateHealthy)
	if len(reasons) > 0 {
		health = string(v1beta1.HealthStateDegraded)
	}
	return map[string]any{"health": health, "reasons": reasons}, nil
}

// nodeIdentity is one node's name and labels (C-36).
type nodeIdentity struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// nodeIdentities reports every node's identity and labels (C-36).
func (r *Readings) nodeIdentities(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeIdentity, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, nodeIdentity{Name: node.Name, Labels: node.Labels})
	}
	return entries, nil
}

// nodeState is one node's readiness and schedulability (C-36).
type nodeState struct {
	Name        string `json:"name"`
	Ready       bool   `json:"ready"`
	Schedulable bool   `json:"schedulable"`
}

// nodeStatuses reports the readiness and schedulability of every node (C-36).
func (r *Readings) nodeStatuses(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeState, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, nodeState{Name: node.Name, Ready: isReady(node), Schedulable: !node.Spec.Unschedulable})
	}
	return entries, nil
}

// nodeConstraints is one node's scheduling constraints (C-36).
type nodeConstraints struct {
	Name        string   `json:"name"`
	Taints      []string `json:"taints,omitempty"`
	Schedulable bool     `json:"schedulable"`
}

// nodeConstraints reports each node's taints and schedulability (C-36).
func (r *Readings) nodeConstraints(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeConstraints, 0, len(nodes))
	for _, node := range nodes {
		taints := make([]string, 0, len(node.Spec.Taints))
		for _, taint := range node.Spec.Taints {
			taints = append(taints, string(taint.Key)+"="+taint.Value+":"+string(taint.Effect))
		}
		entries = append(entries, nodeConstraints{Name: node.Name, Taints: taints, Schedulable: !node.Spec.Unschedulable})
	}
	return entries, nil
}

// nodeCapacity is one node's allocatable CPU and memory (C-36).
type nodeCapacity struct {
	Name   string `json:"name"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// nodeAllocatable reports every node's allocatable resources (C-36, C-71).
func (r *Readings) nodeAllocatable(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeCapacity, 0, len(nodes))
	for _, node := range nodes {
		cpu := allocatable(node, corev1.ResourceCPU)
		memory := allocatable(node, corev1.ResourceMemory)
		entries = append(entries, nodeCapacity{Name: node.Name, CPU: cpu.String(), Memory: memory.String()})
	}
	return entries, nil
}

// nodeDevices is one node's device facts (C-36).
type nodeDevices struct {
	Name     string `json:"name"`
	Devices  int    `json:"devices"`
	Occupied int    `json:"occupied"`
}

// nodeDevices reports the device facts each node reported, with how many are
// occupied (C-37, C-36).
func (r *Readings) nodeDevices(ctx context.Context) (any, *controlplane.Error) {
	objects, failure := r.kernel(ctx, "NodeDevice")
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeDevices, 0, len(objects))
	for _, object := range objects {
		var device v1beta1.NodeDevice
		if failure := decode(object, &device); failure != nil {
			return nil, failure
		}
		entry := nodeDevices{Name: device.Name, Devices: len(device.Spec.Devices)}
		for _, fact := range device.Spec.Devices {
			if fact.Occupancy != nil {
				entry.Occupied++
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name < entries[right].Name })
	return entries, nil
}

// nodeProviders reports the node-level provider status per node: the
// registration is cluster-scoped, so a node reports the providers that declare a
// node-scoped capability or that own a volume bound to it (C-36, C-47).
func (r *Readings) nodeProviders(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	objects, failure := r.kernel(ctx, "Provider")
	if failure != nil {
		return nil, failure
	}
	var nodeLevel []providerSummary
	known := map[string]providerSummary{}
	for _, object := range objects {
		var provider v1beta1.Provider
		if failure := decode(object, &provider); failure != nil {
			return nil, failure
		}
		summary := summaryOf(provider)
		known[provider.Name] = summary
		if len(nodeScoped(provider)) > 0 {
			nodeLevel = append(nodeLevel, summary)
		}
	}
	bound, failure := r.providersByNode(ctx)
	if failure != nil {
		return nil, failure
	}
	type nodeProviders struct {
		Name      string            `json:"name"`
		Providers []providerSummary `json:"providers"`
	}
	entries := make([]nodeProviders, 0, len(nodes))
	for _, node := range nodes {
		providers := append([]providerSummary(nil), nodeLevel...)
		seen := map[string]bool{}
		for _, summary := range providers {
			seen[summary.Name] = true
		}
		for _, name := range bound[node.Name] {
			if seen[name] {
				continue
			}
			seen[name] = true
			if summary, ok := known[name]; ok {
				providers = append(providers, summary)
			}
		}
		if providers == nil {
			providers = []providerSummary{}
		}
		entries = append(entries, nodeProviders{Name: node.Name, Providers: providers})
	}
	return entries, nil
}

// providersByNode maps a node to the provider names of the node-local volumes
// bound to it.
func (r *Readings) providersByNode(ctx context.Context) (map[string][]string, *controlplane.Error) {
	volumes, failure := r.kernel(ctx, "Volume")
	if failure != nil {
		return nil, failure
	}
	providers := map[string][]string{}
	for _, object := range volumes {
		var volume v1beta1.Volume
		if failure := decode(object, &volume); failure != nil {
			return nil, failure
		}
		if volume.Status.Node == "" || volume.Status.Provider.Name == "" {
			continue
		}
		providers[volume.Status.Node] = append(providers[volume.Status.Node], volume.Status.Provider.Name)
	}
	return providers, nil
}

// nodeSchedulability is one node's maintenance state (C-36, C-65).
type nodeSchedulability struct {
	Name        string `json:"name"`
	Schedulable bool   `json:"schedulable"`
}

// nodeMaintenanceStates reports whether each node is unschedulable, which is
// where C-65 keeps the maintenance state.
func (r *Readings) nodeMaintenanceStates(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeSchedulability, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, nodeSchedulability{Name: node.Name, Schedulable: !node.Spec.Unschedulable})
	}
	return entries, nil
}

// nodeOrigin is one node's maintenance origin (C-36).
type nodeOrigin struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
}

// nodeMaintenanceOrigins judges the maintenance origin of every node (C-65):
// the newest of that node's own maintenance records decides. A node with no
// such record that is nevertheless unschedulable was cordoned outside KOS, and
// an unreadable trail is unknown rather than guessed.
func (r *Readings) nodeMaintenanceOrigins(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	origins := make([]nodeOrigin, 0, len(nodes))
	for _, node := range nodes {
		origins = append(origins, nodeOrigin{Name: node.Name, Origin: r.originOf(ctx, node)})
	}
	return origins, nil
}

// originOf applies C-65's origin rule to one node. Only the newest record of
// the node's own maintenance writes that took effect is needed, and the read is
// bounded by that node and that outcome, so neither unrelated audit traffic nor
// a refused attempt changes the answer.
func (r *Readings) originOf(ctx context.Context, node corev1.Node) string {
	if !node.Spec.Unschedulable {
		return MaintenanceNone
	}
	if r.Audit == nil {
		return MaintenanceUnknown
	}
	object := api.Group + "/Node/" + node.Name
	entries, err := r.Audit.RecentAuditEntriesForObject(ctx, object, maintenanceVerbs, maintenanceResults, 1)
	if err != nil {
		return MaintenanceUnknown
	}
	if len(entries) == 0 {
		return MaintenanceUnplanned
	}
	if entries[0].Verb == controlplane.VerbMaintain {
		return MaintenancePlanned
	}
	return MaintenanceUnplanned
}

// nodeApplications is one node and the node-local applications maintenance
// would stop (C-36, C-65).
type nodeApplications struct {
	Name         string   `json:"name"`
	Applications []string `json:"applications"`
}

// nodeMaintenanceAffected computes the node-local applications bound to each
// node: an application is affected when it references a node-local volume whose
// data sits on that node (C-38, C-65).
func (r *Readings) nodeMaintenanceAffected(ctx context.Context) (any, *controlplane.Error) {
	nodes, failure := r.nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	affected, failure := r.affectedByNode(ctx)
	if failure != nil {
		return nil, failure
	}
	entries := make([]nodeApplications, 0, len(nodes))
	for _, node := range nodes {
		names := affected[node.Name]
		if names == nil {
			names = []string{}
		}
		entries = append(entries, nodeApplications{Name: node.Name, Applications: names})
	}
	return entries, nil
}

// affectedByNode maps a node to the applications bound to it through a
// node-local volume. A volume lives in the application's target namespace and is
// addressed by name from there (C-32, C-38), so the lookup pairs the two before
// it compares nodes; applications are named by their object namespace and name.
func (r *Readings) affectedByNode(ctx context.Context) (map[string][]string, *controlplane.Error) {
	volumes, failure := r.kernel(ctx, "Volume")
	if failure != nil {
		return nil, failure
	}
	bound := map[string]string{}
	for _, object := range volumes {
		var volume v1beta1.Volume
		if failure := decode(object, &volume); failure != nil {
			return nil, failure
		}
		if volume.Spec.Mobility != v1beta1.VolumeMobilityNodeLocal || volume.Status.Node == "" {
			continue
		}
		bound[volume.Namespace+"/"+volume.Name] = volume.Status.Node
	}
	applications, failure := r.kernel(ctx, "Application")
	if failure != nil {
		return nil, failure
	}
	affected := map[string][]string{}
	for _, object := range applications {
		var application v1beta1.Application
		if failure := decode(object, &application); failure != nil {
			return nil, failure
		}
		namespace := application.Namespace
		if application.Spec.Namespace != "" {
			namespace = application.Spec.Namespace
		}
		nodes := map[string]bool{}
		for _, name := range application.Spec.Volumes {
			if node, ok := bound[namespace+"/"+name]; ok {
				nodes[node] = true
			}
		}
		for node := range nodes {
			affected[node] = append(affected[node], application.Namespace+"/"+application.Name)
		}
	}
	for node := range affected {
		sort.Strings(affected[node])
	}
	return affected, nil
}

// settingValue reads one declared setting's current value through the settings
// domain (C-23). A declared value the user never wrote reads as empty.
func (r *Readings) settingValue(ctx context.Context, session identity.Session, id string) (any, *controlplane.Error) {
	if r.Settings == nil {
		return nil, unreadable("设置域不可用")
	}
	items, failure := r.Settings.Items(ctx, session, "")
	if failure != nil {
		return nil, failure
	}
	for _, item := range items {
		if item.ID == id {
			return item.Value, nil
		}
	}
	return "", nil
}

// nodes reads every node ordered by name.
func (r *Readings) nodes(ctx context.Context) ([]corev1.Node, *controlplane.Error) {
	objects, failure := r.list(ctx, schema.GroupVersionResource{Version: "v1", Resource: "nodes"})
	if failure != nil {
		return nil, failure
	}
	nodes := make([]corev1.Node, 0, len(objects))
	for _, object := range objects {
		var node corev1.Node
		if failure := decode(object, &node); failure != nil {
			return nil, failure
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(left, right int) bool { return nodes[left].Name < nodes[right].Name })
	return nodes, nil
}

// reader returns the kernel reader the C-59 predicates run on: the same client
// and clock the item's own read uses, so one page judges one cluster.
func (r *Readings) reader() *cluster.Reader {
	return &cluster.Reader{Dynamic: r.Dynamic, Now: r.Now}
}

// kernel reads one kernel kind's objects; an empty namespace reads every one.
func (r *Readings) kernel(ctx context.Context, kind string) ([]unstructured.Unstructured, *controlplane.Error) {
	resource, ok := api.ResourceName(kind)
	if !ok {
		return nil, unreadable(kind + " 不是内核对象类型")
	}
	return r.list(ctx, schema.GroupVersionResource{
		Group:    api.Group,
		Version:  v1beta1.GroupVersion.Version,
		Resource: resource,
	})
}

// list reads one resource; an empty namespace lists every namespace.
func (r *Readings) list(ctx context.Context, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, *controlplane.Error) {
	if r.Dynamic == nil {
		return nil, clusterUnavailable()
	}
	list, err := r.Dynamic.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, clusterUnavailable()
	}
	return list.Items, nil
}

// decode converts one stored object into its typed shape.
func decode(object unstructured.Unstructured, target any) *controlplane.Error {
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, target); err != nil {
		return unreadable("对象 " + object.GetName() + " 无法解析")
	}
	return nil
}

// rolesOf reports the roles a node's labels declare.
func rolesOf(node corev1.Node) []string {
	const prefix = "node-role.kubernetes.io/"
	roles := make([]string, 0, 2)
	for label := range node.Labels {
		if role, ok := strings.CutPrefix(label, prefix); ok && role != "" {
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	return roles
}

// isReady reports the node's Ready condition.
func isReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// allocatable reads one allocatable resource of a node.
func allocatable(node corev1.Node, name corev1.ResourceName) apiresource.Quantity {
	return node.Status.Allocatable[name]
}

// summaryOf renders one registered provider.
func summaryOf(provider v1beta1.Provider) providerSummary {
	summary := providerSummary{
		Name:           provider.Name,
		Version:        provider.Spec.Version,
		Health:         string(provider.Status.Health.State),
		StorageClasses: provider.Spec.StorageClasses,
	}
	for _, capability := range provider.Spec.Capabilities {
		summary.Capabilities = append(summary.Capabilities, capability.Name+"@"+capability.Version)
	}
	return summary
}

// nodeScoped returns the capabilities of one provider that hold only on the
// nodes providing them (C-09).
func nodeScoped(provider v1beta1.Provider) []v1beta1.ProviderCapability {
	capabilities := make([]v1beta1.ProviderCapability, 0, len(provider.Spec.Capabilities))
	for _, capability := range provider.Spec.Capabilities {
		if capability.Scope == v1beta1.CapabilityScopeNode {
			capabilities = append(capabilities, capability)
		}
	}
	return capabilities
}

// now supplies the read clock.
func (r *Readings) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// unreadable reports an item whose source cannot be judged.
func unreadable(message string) *controlplane.Error {
	return &controlplane.Error{
		Code:        controlplane.CodeInternal,
		Message:     message,
		Stage:       controlplane.StageRead,
		SideEffects: controlplane.SideEffectsNone,
	}
}

// clusterUnavailable reports a cluster read that failed (C-56).
func clusterUnavailable() *controlplane.Error {
	return &controlplane.Error{
		Code:        controlplane.CodeClusterUnavailable,
		Message:     "集群暂不可达，设置读取失败",
		Stage:       controlplane.StageRead,
		Retryable:   true,
		SideEffects: controlplane.SideEffectsNone,
	}
}
