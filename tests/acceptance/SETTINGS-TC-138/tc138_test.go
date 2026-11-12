// Package settingstc138 verifies that every settings item is judged by one
// source type: an object-source node item follows the node object, a
// database-source policy item follows the control-plane database and not the
// object, and the page states the reason when the database stops answering
// (SETTINGS-AC-31, SETTINGS-TC-138).
package settingstc138

import (
	"context"
	"testing"
	"time"

	"github.com/tsic404/kosd/pkg/controlplane"

	"github.com/tsic404/kos-settings/pkg/kossettings"
	"github.com/tsic404/kos-settings/tests/acceptance/fixtures"
)

func TestTC138ObjectAndDatabaseSources(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Node(t, cluster, "node-a", "4", "8Gi", true, false, map[string]string{"node-role.kubernetes.io/worker": ""})
	fixtures.Node(t, cluster, "node-b", "2", "4Gi", true, false, nil)

	values := fixtures.NewValues()
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, values)

	// Write the cluster policy through the domain the page's entry names, then
	// read the page: the policy item reports that value from the database and
	// the node item reports the node object.
	const window = "Sun 02:00-04:00"
	if _, failure := shell.Settings.Set(context.Background(), fixtures.Owner, kossettings.ClusterMaintenanceWindow, "", window); failure != nil {
		t.Fatalf("write the cluster policy: %s (%s)", failure.Message, failure.Code)
	}

	before := fixtures.ItemsByID(t, fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings))
	if value := fixtures.String(before[kossettings.ClusterMaintenanceWindow], "value"); value != window {
		t.Fatalf("policy value = %q, want the written window", value)
	}
	if value := fixtures.NodeField(t, before, kossettings.NodeStatus, "node-a", "schedulable"); value != true {
		t.Fatalf("node-a schedulable = %v, want true before the object changes", value)
	}

	// Change the node object directly; the object-source item follows it, the
	// database-source policy item does not (C-36, C-53).
	fixtures.Cordone(t, cluster, "node-a")
	after := fixtures.ItemsByID(t, fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings))
	if value := fixtures.NodeField(t, after, kossettings.NodeStatus, "node-a", "schedulable"); value != false {
		t.Fatalf("node-a schedulable after the cordon = %v, want the object's new value", value)
	}
	if value := fixtures.String(after[kossettings.ClusterMaintenanceWindow], "value"); value != window {
		t.Fatalf("policy value after an object change = %q, want the unchanged database value", value)
	}

	// Each item is judged by one source type only: it appears once and its
	// source type is the one the contract declares.
	if len(after) != len(fixtures.SettingsPageItems) {
		t.Fatalf("the page renders %d items, want %d", len(after), len(fixtures.SettingsPageItems))
	}
	for _, expected := range fixtures.SettingsPageItems {
		item, found := after[expected.ID]
		if !found {
			t.Fatalf("item %q is missing from the page", expected.ID)
		}
		if source := fixtures.String(item, "source"); source != expected.Source {
			t.Fatalf("item %q source = %q, want %q", expected.ID, source, expected.Source)
		}
	}

	// The control-plane database stops answering: the page states the reason
	// instead of showing an invented value (C-61).
	values.Down()
	failure := fixtures.FailureOf(shell, fixtures.Owner, kossettings.PageSettings)
	if failure == nil {
		t.Fatal("the settings page answered while the control-plane database was down")
	}
	if failure.Code != controlplane.CodeDatabaseUnavailable {
		t.Fatalf("database outage code = %q, want %q", failure.Code, controlplane.CodeDatabaseUnavailable)
	}
	if failure.Message == "" {
		t.Fatal("database outage carries no reason")
	}

	// With the database back the same item reads the stored value again.
	values.Up()
	restored := fixtures.ItemsByID(t, fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings))
	if value := fixtures.String(restored[kossettings.ClusterMaintenanceWindow], "value"); value != window {
		t.Fatalf("policy value after recovery = %q, want the retained value", value)
	}
}
