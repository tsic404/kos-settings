// Package settingstc135 verifies the four states of the resource-occupancy
// items: allocatable capacity stays readable while sampled usage appears only
// when the metric source covers every node, and otherwise states why it is
// unavailable, naming the uncovered nodes, with no usage at any level
// (SETTINGS-AC-64, SETTINGS-TC-135, SETTINGS-C-71).
package settingstc135

import (
	"strings"
	"testing"
	"time"

	"github.com/tsic404/kos-settings/pkg/kossettings"
	"github.com/tsic404/kos-settings/tests/acceptance/fixtures"
	"k8s.io/apimachinery/pkg/api/resource"
)

// cluster is the two-node cluster the cases read: the samples cover it whole,
// then stop covering it.
const (
	nodeA = "node-a"
	nodeB = "node-b"
)

// page reads the settings page and indexes its projected items.
func page(t *testing.T, shell *fixtures.Shell) map[string]map[string]any {
	t.Helper()
	return fixtures.ItemsByID(t, fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings))
}

// valueOf returns one item's projected value as an object.
func valueOf(t *testing.T, items map[string]map[string]any, id string) map[string]any {
	t.Helper()
	item, found := items[id]
	if !found {
		t.Fatalf("page has no item %q", id)
	}
	value, ok := item["value"].(map[string]any)
	if !ok {
		t.Fatalf("item %q value = %v, want an object", id, item["value"])
	}
	return value
}

// quantityOf reads one quantity field of a projected object and fails the case
// when it is absent or unparsable.
func quantityOf(t *testing.T, object map[string]any, keys ...string) resource.Quantity {
	t.Helper()
	text, ok := fixtures.Nested(object, keys...).(string)
	if !ok || text == "" {
		t.Fatalf("%v = %v, want a quantity", keys, fixtures.Nested(object, keys...))
	}
	quantity, err := resource.ParseQuantity(text)
	if err != nil {
		t.Fatalf("%v = %q is not a quantity: %v", keys, text, err)
	}
	return quantity
}

// assertQuantity checks one field equals the expected quantity.
func assertQuantity(t *testing.T, object map[string]any, want string, keys ...string) {
	t.Helper()
	expected := resource.MustParse(want)
	if actual := quantityOf(t, object, keys...); actual.Cmp(expected) != 0 {
		t.Fatalf("%v = %s, want %s", keys, actual.String(), expected.String())
	}
}

// assertCapacityReadable checks that both occupancy groups report the nodes'
// allocatable capacity, the state the contract keeps in all four states
// (C-71).
func assertCapacityReadable(t *testing.T, items map[string]map[string]any) {
	t.Helper()
	cluster := valueOf(t, items, kossettings.ClusterAllocatable)
	assertQuantity(t, cluster, "6", "allocatable", "cpu")
	assertQuantity(t, cluster, "12Gi", "allocatable", "memory")

	entries, ok := items[kossettings.NodeAllocatable]["value"].([]any)
	if !ok {
		t.Fatalf("node capacity value = %v, want a per-node list", items[kossettings.NodeAllocatable]["value"])
	}
	want := map[string][2]string{
		nodeA: {"4", "8Gi"},
		nodeB: {"2", "4Gi"},
	}
	for _, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("node capacity entry = %v, want an object", entry)
		}
		name := fixtures.String(record, "name")
		expected, known := want[name]
		if !known {
			t.Fatalf("node capacity reports the unknown node %q: %v", name, entries)
		}
		assertQuantity(t, record, expected[0], "cpu")
		assertQuantity(t, record, expected[1], "memory")
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("node capacity left the nodes %v unsampled: %v", want, entries)
	}
}

// assertNoUsage checks that an unavailable item carries no usage at any level:
// no cluster total, no per-node entry and no quantity anywhere in it (C-71).
func assertNoUsage(t *testing.T, value map[string]any) {
	t.Helper()
	for _, field := range []string{"used", "nodes"} {
		if _, present := value[field]; present {
			t.Fatalf("unavailable item carries %q: %v", field, value)
		}
	}
	if key := usageKeyIn(value); key != "" {
		t.Fatalf("unavailable item carries the usage field %q: %v", key, value)
	}
}

// usageKeyIn returns the first consumption field found anywhere in the value,
// so an unavailable item is judged whatever level it hides usage at.
func usageKeyIn(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == "cpu" || key == "memory" || key == "used" {
				return key
			}
			if found := usageKeyIn(nested); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := usageKeyIn(nested); found != "" {
				return found
			}
		}
	}
	return ""
}

// assertUnavailable checks that an occupancy item states it is unavailable with
// a reason that names the uncovered nodes and no usage anywhere (C-71).
func assertUnavailable(t *testing.T, items map[string]map[string]any, id string, uncovered ...string) {
	t.Helper()
	value := valueOf(t, items, id)
	if value["available"] != false {
		t.Fatalf("item %q value = %v, want an unavailable item", id, value)
	}
	reason, _ := value["reason"].(string)
	if reason == "" {
		t.Fatalf("item %q carries no reason: %v", id, value)
	}
	for _, node := range uncovered {
		if !strings.Contains(reason, node) {
			t.Fatalf("item %q reason = %q, want it to name the uncovered node %q", id, reason, node)
		}
	}
	assertNoUsage(t, value)
}

// TestTC135ResourceUsageStates drives one controllable metric source through
// the four states of SETTINGS-TC-135 and reads the resource-occupancy items
// after each, judging the state the page presents (SETTINGS-AC-64).
func TestTC135ResourceUsageStates(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Node(t, cluster, nodeA, "4", "8Gi", true, false, nil)
	fixtures.Node(t, cluster, nodeB, "2", "4Gi", true, false, nil)

	usage := fixtures.NewUsage(
		fixtures.Sample(nodeA, "1", "1Gi"),
		fixtures.Sample(nodeB, "2", "2Gi"),
	)
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShellWithUsage(t, registry, cluster, fixtures.NewValues(), usage)

	// Full coverage: both levels report the samples, and the cluster total is
	// their sum over every node (C-71).
	items := page(t, shell)
	assertCapacityReadable(t, items)

	clusterUsage := valueOf(t, items, kossettings.ClusterUsage)
	if clusterUsage["available"] != true {
		t.Fatalf("cluster usage value = %v, want the sampled usage", clusterUsage)
	}
	assertQuantity(t, clusterUsage, "3", "used", "cpu")
	assertQuantity(t, clusterUsage, "3Gi", "used", "memory")
	// The cluster item nests each node's sample under "used".
	assertSamples(t, clusterUsage, func(record map[string]any) map[string]any {
		used, _ := record["used"].(map[string]any)
		return used
	}, map[string][2]string{
		nodeA: {"1", "1Gi"},
		nodeB: {"2", "2Gi"},
	})

	nodeUsage := valueOf(t, items, kossettings.NodeUsage)
	if nodeUsage["available"] != true {
		t.Fatalf("node usage value = %v, want the sampled usage", nodeUsage)
	}
	assertSamples(t, nodeUsage, func(record map[string]any) map[string]any { return record }, map[string][2]string{
		nodeA: {"1", "1Gi"},
		nodeB: {"2", "2Gi"},
	})

	// An empty sample window covers no node: the item is unavailable, the
	// reason names every uncovered node, and no level shows usage (C-71).
	usage.Set()
	items = page(t, shell)
	assertCapacityReadable(t, items)
	assertUnavailable(t, items, kossettings.ClusterUsage, nodeA, nodeB)
	assertUnavailable(t, items, kossettings.NodeUsage, nodeA, nodeB)

	// One node sampled out of two is still an uncovered node, and its single
	// sample is not the cluster's usage (C-71).
	usage.Set(fixtures.Sample(nodeA, "1", "1Gi"))
	items = page(t, shell)
	assertCapacityReadable(t, items)
	assertUnavailable(t, items, kossettings.ClusterUsage, nodeB)
	assertUnavailable(t, items, kossettings.NodeUsage, nodeB)

	// A distribution that integrates no metric source states that, and the
	// capacity stays readable beside it (C-71).
	shell.DetachMetricSource()
	items = page(t, shell)
	assertCapacityReadable(t, items)
	assertUnavailable(t, items, kossettings.ClusterUsage)
	assertUnavailable(t, items, kossettings.NodeUsage)
}

// assertSamples checks that a usable occupancy item reports exactly the given
// nodes' samples and that the cluster total is their sum. sampleOf returns the
// record carrying one node's CPU and memory, which the two occupancy items
// shape differently.
func assertSamples(t *testing.T, value map[string]any, sampleOf func(map[string]any) map[string]any, want map[string][2]string) {
	t.Helper()
	entries, ok := value["nodes"].([]any)
	if !ok {
		t.Fatalf("usage value = %v, want a per-node list", value)
	}
	if len(entries) != len(want) {
		t.Fatalf("usage reports %d nodes, want %d: %v", len(entries), len(want), entries)
	}
	totalCPU := resource.Quantity{}
	totalMemory := resource.Quantity{}
	remaining := make(map[string][2]string, len(want))
	for name, sample := range want {
		remaining[name] = sample
	}
	for _, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("usage entry = %v, want an object", entry)
		}
		name := fixtures.String(record, "name")
		expected, known := remaining[name]
		if !known {
			t.Fatalf("usage reports the node %q, want only %v: %v", name, want, entries)
		}
		delete(remaining, name)

		sample := sampleOf(record)
		if sample == nil {
			t.Fatalf("usage entry for node %q carries no sample: %v", name, record)
		}
		cpu := quantityOf(t, sample, "cpu")
		memory := quantityOf(t, sample, "memory")
		totalCPU.Add(cpu)
		totalMemory.Add(memory)
		if cpu.Cmp(resource.MustParse(expected[0])) != 0 || memory.Cmp(resource.MustParse(expected[1])) != 0 {
			t.Fatalf("node %q sample = %s/%s, want %s/%s", name, cpu.String(), memory.String(), expected[0], expected[1])
		}
	}
	if len(remaining) != 0 {
		t.Fatalf("usage left the nodes %v unreported: %v", remaining, entries)
	}
	// The cluster total is the sum of the node samples, so it is present only
	// when the item reports a cluster level.
	if used, present := value["used"].(map[string]any); present {
		usedCPU := quantityOf(t, used, "cpu")
		usedMemory := quantityOf(t, used, "memory")
		if usedCPU.Cmp(totalCPU) != 0 || usedMemory.Cmp(totalMemory) != 0 {
			t.Fatalf("cluster total = %v, want the sample sum %s/%s", used, totalCPU.String(), totalMemory.String())
		}
	}
}
