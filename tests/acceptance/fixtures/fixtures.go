// Package fixtures boots the pieces the settings cases need: the pure kernel
// install model (KOSD-C-17, KOSD-C-28), a throwaway cluster object set, the
// settings domain's value store, and the settings page shell. Nothing here
// reaches a real cluster or database.
package fixtures

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/settings"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-settings/pkg/kossettings"
)

// Owner is the account the cases read and write with.
var Owner = identity.Session{Username: "owner", Role: write.RoleOwner}

// Member is a signed-in account without the Owner role.
var Member = identity.Session{Username: "member", Role: write.RoleMember}

// Guest is a signed-in account with the Guest role.
var Guest = identity.Session{Username: "guest", Role: write.RoleGuest}

// ItemExpectation is one display item as SETTINGS-C-36 declares it: the source
// type that judges it and whether the user may modify it.
type ItemExpectation struct {
	ID         string
	Source     string
	Modifiable bool
}

// SettingsPageItems is the frozen list SETTINGS-C-36 puts in the cluster and
// node groups, in declaration order. Capacity and sampled usage stay separate
// entries because each has its own source type (C-36, C-53, C-71).
var SettingsPageItems = []ItemExpectation{
	{kossettings.ClusterNodesAndRoles, "object", false},
	{kossettings.ClusterSystemComponents, "object", false},
	{kossettings.ClusterNetworkAndStorage, "object", false},
	{kossettings.ClusterVersion, "computed", false},
	{kossettings.ClusterCapabilities, "object", false},
	{kossettings.ClusterHealth, "computed", false},
	{kossettings.ClusterMaintenanceWindow, "database", true},
	{kossettings.ClusterAllocatable, "object", false},
	{kossettings.ClusterUsage, "metric", false},
	{kossettings.NodeIdentity, "object", false},
	{kossettings.NodeStatus, "object", false},
	{kossettings.NodeSchedulingConstraints, "object", false},
	{kossettings.NodeDevices, "object", false},
	{kossettings.NodeProviderStatus, "object", false},
	{kossettings.NodeAllocatable, "object", false},
	{kossettings.NodeUsage, "metric", false},
	{kossettings.NodeMaintenanceState, "object", false},
	{kossettings.NodeMaintenanceAffected, "computed", false},
	{kossettings.NodeMaintenanceOrigin, "database", false},
	{kossettings.NodeLevelConfiguration, "read", false},
}

// DeclarationContent is the declaration document one install request carries
// (KOSD-C-17).
func DeclarationContent(t *testing.T) []byte {
	t.Helper()
	content, err := json.Marshal(kossettings.Declaration())
	if err != nil {
		t.Fatalf("marshal the product declaration: %v", err)
	}
	return content
}

// DeclarationMap is the product declaration as a JSON object.
func DeclarationMap(t *testing.T) map[string]any {
	t.Helper()
	var declaration map[string]any
	if err := json.Unmarshal(DeclarationContent(t), &declaration); err != nil {
		t.Fatalf("decode the product declaration: %v", err)
	}
	return declaration
}

// Product returns one install request carrying the product declaration.
func Product(t *testing.T, name string, created time.Time) extension.Request {
	t.Helper()
	content := DeclarationContent(t)
	return extension.Request{
		Name:    name,
		Created: created,
		Package: &extension.Package{
			Reference: name + "-package",
			Digest:    extension.DigestOf(content),
			Content:   content,
		},
	}
}

// OtherProduct is a second extension the isolation cases install beside
// kos-settings: one page and one setting, nothing shared.
func OtherProduct(t *testing.T, created time.Time) extension.Request {
	t.Helper()
	declaration := extension.Declaration{
		Identifier:       "other-product",
		Version:          "1.0.0",
		FrameworkVersion: ">=1.0 <2.0",
		Contributions: extension.Contributions{
			Pages: []extension.Page{{
				ID:             "other-page",
				Title:          "其他产品",
				Location:       "panel",
				ReadOnly:       true,
				ReadOnlyReason: "本页只读",
				Items: []extension.PageItem{{
					ID: "other-item", Label: "其他项", Source: extension.PageSourceObject,
					Reference: "other-product/other-item",
				}},
			}},
			Settings: []extension.Setting{{
				ID: "other-setting", Scope: extension.SettingScopeCluster,
				Writer: extension.SettingWriterUser, Editable: true,
			}},
		},
	}
	content, err := json.Marshal(declaration)
	if err != nil {
		t.Fatalf("marshal the other product declaration: %v", err)
	}
	return extension.Request{
		Name:    "other",
		Created: created,
		Package: &extension.Package{
			Reference: "other-package",
			Digest:    extension.DigestOf(content),
			Content:   content,
		},
	}
}

// Install evaluates the live install requests with the kernel's own rules and
// returns the contribution projection they produce (KOSD-C-17, KOSD-C-28).
func Install(t *testing.T, requests ...extension.Request) *extension.Registry {
	t.Helper()
	return extension.Evaluate(requests).Registry
}

// Values is the settings domain's value store. An in-memory map stands in for
// the control-plane database; Err makes every read and write fail the way a
// stopped database does (C-61).
type Values struct {
	// Err is returned by every operation while set.
	Err error
	// data is keyed by scope, target and item.
	data map[string]map[string]string
}

// NewValues returns an empty store.
func NewValues() *Values {
	return &Values{data: map[string]map[string]string{}}
}

// Down makes the store answer like a stopped control-plane database.
func (v *Values) Down() { v.Err = errors.New("database is not answering") }

// Up restores the store.
func (v *Values) Up() { v.Err = nil }

// SettingValues implements settings.Values.
func (v *Values) SettingValues(_ context.Context, scope, target string) (map[string]string, error) {
	if v.Err != nil {
		return nil, v.Err
	}
	return v.data[scope+"\x00"+target], nil
}

// SetSettingValue implements settings.Values.
func (v *Values) SetSettingValue(_ context.Context, scope, target, identifier, value string) error {
	if v.Err != nil {
		return v.Err
	}
	key := scope + "\x00" + target
	if v.data[key] == nil {
		v.data[key] = map[string]string{}
	}
	v.data[key][identifier] = value
	return nil
}

// Roster is the control-plane database's account table. Err makes every read
// fail the way a stopped database does (C-61); the accounts themselves are what
// the user page's database item reports.
type Roster struct {
	// Err is returned by every read while set.
	Err error
	// accounts is the table the page reads.
	accounts []identity.Account
}

// NewRoster returns a database holding the given accounts.
func NewRoster(accounts ...identity.Account) *Roster {
	return &Roster{accounts: accounts}
}

// Down makes the roster answer like a stopped control-plane database.
func (r *Roster) Down() { r.Err = errors.New("database is not answering") }

// Up restores the roster.
func (r *Roster) Up() { r.Err = nil }

// Add appends one account to the table.
func (r *Roster) Add(account identity.Account) { r.accounts = append(r.accounts, account) }

// ListAccounts implements kossettings.MemberSource.
func (r *Roster) ListAccounts(_ context.Context) ([]identity.Account, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return append([]identity.Account(nil), r.accounts...), nil
}

// Shell is one installed product's page and settings projection over the same
// object set and value store.
type Shell struct {
	// Cluster is the object set the page sources read.
	Cluster dynamic.Interface
	// Values is the control-plane database stand-in.
	Values *Values
	// Settings is the kernel's settings domain over this product's
	// declarations.
	Settings *settings.Service
	// Pages is the shell that renders the declared pages (C-22, C-53).
	Pages *pages.Service
}

// NewShell wires the settings domain and the page shell over one registry and
// object set. The product's read model answers every declared server source;
// the usage items state that no metric source is configured. A roster, when
// given, is the control-plane database's account table the user page reads;
// without one the page states that the database holds no account it can show.
func NewShell(t *testing.T, registry *extension.Registry, cluster dynamic.Interface, values *Values, roster ...*Roster) *Shell {
	t.Helper()
	service := settings.New(registry, values, settings.Config{})
	members := NewRoster()
	if len(roster) > 0 && roster[0] != nil {
		members = roster[0]
	}
	readings := &kossettings.Readings{
		Dynamic:  cluster,
		Settings: service,
		Members:  members,
		Now:      func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) },
	}
	return &Shell{
		Cluster:  cluster,
		Values:   values,
		Settings: service,
		Pages:    pages.New(registry, pages.Config{Readings: pages.Router{kossettings.Identifier: readings}}),
	}
}

// ObjectSet builds a fake dynamic client holding the kernel objects the cases
// shape. The scheme registers the objects as unstructured so the fake stores
// exactly the JSON the page sources read, without a typed round trip.
func ObjectSet(t *testing.T) dynamic.Interface {
	t.Helper()
	scheme := runtime.NewScheme()
	lists := map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "nodes"}: "NodeList",
	}
	add := func(gvk schema.GroupVersionKind, resource string) {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
		lists[gvk.GroupVersion().WithResource(resource)] = gvk.Kind + "List"
	}
	add(schema.GroupVersionKind{Version: "v1", Kind: "Node"}, "nodes")
	for _, kind := range []string{"Application", "Provider", "SystemComponent", "NodeDevice", "Volume"} {
		resource, ok := api.ResourceName(kind)
		if !ok {
			t.Fatalf("kernel kind %q has no resource name", kind)
		}
		add(schema.GroupVersionKind{Group: api.Group, Version: v1beta1.GroupVersion.Version, Kind: kind}, resource)
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, lists)
}

// Node writes one node with the status the settings sources read.
func Node(t *testing.T, cluster dynamic.Interface, name, cpu, memory string, ready, unschedulable bool, labels map[string]string) {
	t.Helper()
	conditions := []any{map[string]any{"type": "Ready", "status": "False"}}
	if ready {
		conditions = []any{map[string]any{"type": "Ready", "status": "True"}}
	}
	recorded := make(map[string]any, len(labels))
	for key, value := range labels {
		recorded[key] = value
	}
	object := map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": name, "labels": recorded},
		"spec":       map[string]any{"unschedulable": unschedulable},
		"status": map[string]any{
			"conditions":  conditions,
			"allocatable": map[string]any{"cpu": cpu, "memory": memory},
			"nodeInfo":    map[string]any{"kubeletVersion": "v1.37.0"},
		},
	}
	create(t, cluster, schema.GroupVersionResource{Version: "v1", Resource: "nodes"}, object)
}

// Cordone rewrites one node's unschedulable field, the way a maintenance start
// or a manual cordon does (C-65).
func Cordone(t *testing.T, cluster dynamic.Interface, name string) {
	t.Helper()
	resource := schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	node, err := cluster.Resource(resource).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read node %q: %v", name, err)
	}
	node.Object["spec"] = map[string]any{"unschedulable": true}
	if _, err := cluster.Resource(resource).Update(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("cordon node %q: %v", name, err)
	}
}

// Application writes one application with the owner and visibility the user
// page reads (C-1, C-59, C-60).
func Application(t *testing.T, cluster dynamic.Interface, namespace, name, owner string, members []string, allowGuests bool) {
	t.Helper()
	visibility := map[string]any{"allowGuests": allowGuests}
	listed := make([]any, 0, len(members))
	for _, member := range members {
		listed = append(listed, member)
	}
	if len(listed) > 0 {
		visibility["members"] = listed
	}
	resource, _ := api.ResourceName("Application")
	createIn(t, cluster, schema.GroupVersionResource{
		Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: resource,
	}, namespace, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "Application",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"owner": owner, "visibility": visibility, "namespace": namespace, "definitionRef": name,
		},
	})
}

// Volume writes one volume with the owner the user page reads (C-1, C-60).
func Volume(t *testing.T, cluster dynamic.Interface, namespace, name, owner string) {
	t.Helper()
	resource, _ := api.ResourceName("Volume")
	createIn(t, cluster, schema.GroupVersionResource{
		Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: resource,
	}, namespace, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "Volume",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       map[string]any{"owner": owner, "mobility": "node-local", "capacity": "1Gi"},
	})
}

// SetApplicationVisibility rewrites one application's visibility, the way the
// owner or the global Owner does (C-59).
func SetApplicationVisibility(t *testing.T, cluster dynamic.Interface, namespace, name string, members []string, allowGuests bool) {
	t.Helper()
	listed := make([]any, 0, len(members))
	for _, member := range members {
		listed = append(listed, member)
	}
	updateSpec(t, cluster, "Application", namespace, name, "visibility", map[string]any{
		"members": listed, "allowGuests": allowGuests,
	})
}

// SetObjectOwner rewrites one owned object's owner, the way the global Owner's
// transfer does (C-60).
func SetObjectOwner(t *testing.T, cluster dynamic.Interface, kind, namespace, name, owner string) {
	t.Helper()
	updateSpec(t, cluster, kind, namespace, name, "owner", owner)
}

// updateSpec rewrites one field of a kernel object's spec.
func updateSpec(t *testing.T, cluster dynamic.Interface, kind, namespace, name, field string, value any) {
	t.Helper()
	resource, ok := api.ResourceName(kind)
	if !ok {
		t.Fatalf("kernel kind %q has no resource name", kind)
	}
	gvr := schema.GroupVersionResource{Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: resource}
	object, err := cluster.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read %s %s/%s: %v", kind, namespace, name, err)
	}
	if err := unstructured.SetNestedField(object.Object, value, "spec", field); err != nil {
		t.Fatalf("set %s.%s of %s/%s: %v", field, kind, namespace, name, err)
	}
	if _, err := cluster.Resource(gvr).Namespace(namespace).Update(context.Background(), object, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update %s %s/%s: %v", kind, namespace, name, err)
	}
}

// Capability is one capability a provider registers and the result KOS
// verified it with (C-09, C-10).
type Capability struct {
	Name    string
	Version string
	Scope   string
	State   string
}

// Provider writes one registered provider with its verification results.
func Provider(t *testing.T, cluster dynamic.Interface, name string, capabilities ...Capability) {
	t.Helper()
	registered := make([]any, 0, len(capabilities))
	results := make([]any, 0, len(capabilities))
	for _, capability := range capabilities {
		registered = append(registered, map[string]any{
			"name": capability.Name, "version": capability.Version, "scope": capability.Scope,
			"caseSet": "cases", "caseSetDigest": "sha256:cases",
		})
		results = append(results, map[string]any{
			"name": capability.Name, "version": capability.Version,
			"source": "KOS", "state": capability.State,
		})
	}
	resource, _ := api.ResourceName("Provider")
	create(t, cluster, schema.GroupVersionResource{Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: resource}, map[string]any{
		"apiVersion": api.Group + "/v1beta1",
		"kind":       "Provider",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"version":        "1.2.0",
			"storageClasses": []any{"standard"},
			"capabilities":   registered,
		},
		"status": map[string]any{
			"health":       map[string]any{"state": "Healthy"},
			"capabilities": results,
		},
	})
}

// create stores one object in the fake client.
func create(t *testing.T, cluster dynamic.Interface, resource schema.GroupVersionResource, object map[string]any) {
	t.Helper()
	if _, err := cluster.Resource(resource).Create(context.Background(), &unstructured.Unstructured{Object: object}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create %s: %v", resource.Resource, err)
	}
}

// createIn stores one namespaced object in the fake client; the client refuses a
// namespace the resource call does not name.
func createIn(t *testing.T, cluster dynamic.Interface, resource schema.GroupVersionResource, namespace string, object map[string]any) {
	t.Helper()
	if _, err := cluster.Resource(resource).Namespace(namespace).Create(context.Background(), &unstructured.Unstructured{Object: object}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create %s in %q: %v", resource.Resource, namespace, err)
	}
}

// ReadPage renders one declared page for the session and fails the case when
// the shell refuses it.
func ReadPage(t *testing.T, shell *Shell, session identity.Session, id string) pages.Page {
	t.Helper()
	page, failure := shell.Pages.Read(context.Background(), session, id)
	if failure != nil {
		t.Fatalf("read page %q: %s (%s)", id, failure.Message, failure.Code)
	}
	return page
}

// ItemOf returns one rendered item by identity.
func ItemOf(t *testing.T, page pages.Page, id string) pages.Item {
	t.Helper()
	for _, item := range page.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("page %q has no item %q", page.ID, id)
	return pages.Item{}
}

// JSONOf renders one page the way the control-plane API serializes it, so a
// case judges the projection a browser receives instead of an internal type.
func JSONOf(t *testing.T, page pages.Page) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("encode page %q: %v", page.ID, err)
	}
	var projected map[string]any
	if err := json.Unmarshal(encoded, &projected); err != nil {
		t.Fatalf("decode page %q: %v", page.ID, err)
	}
	return projected
}

// ItemsByID indexes one page's rendered items by identity.
func ItemsByID(t *testing.T, page pages.Page) map[string]map[string]any {
	t.Helper()
	projected := JSONOf(t, page)
	raw, ok := projected["items"].([]any)
	if !ok {
		t.Fatalf("page %q carries no items: %v", page.ID, projected)
	}
	items := make(map[string]map[string]any, len(raw))
	for index, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("page %q item %d is not an object: %v", page.ID, index, entry)
		}
		id, _ := item["id"].(string)
		if _, duplicated := items[id]; duplicated {
			t.Fatalf("page %q renders item %q twice", page.ID, id)
		}
		items[id] = item
	}
	return items
}

// String reads a non-empty string field of one projected object.
func String(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

// Nested reads a nested field of one projected object.
func Nested(object map[string]any, keys ...string) any {
	var current any = object
	for _, key := range keys {
		record, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = record[key]
	}
	return current
}

// NodeField reads one field of the entry a per-node item reports for a node.
func NodeField(t *testing.T, items map[string]map[string]any, id, node, field string) any {
	t.Helper()
	item, found := items[id]
	if !found {
		t.Fatalf("page has no item %q", id)
	}
	entries, ok := item["value"].([]any)
	if !ok {
		t.Fatalf("item %q value is not a per-node list: %v", id, item["value"])
	}
	for _, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if record["name"] == node {
			return record[field]
		}
	}
	t.Fatalf("item %q has no entry for node %q: %v", id, node, entries)
	return nil
}

// FailureOf renders one page and returns the refusal instead of failing, so a
// case can judge a source that is meant to be unavailable.
func FailureOf(shell *Shell, session identity.Session, id string) *controlplane.Error {
	_, failure := shell.Pages.Read(context.Background(), session, id)
	return failure
}
