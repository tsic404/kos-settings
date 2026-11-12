// Package settingstc136 verifies the cluster maintenance window as a policy
// item: it is cluster-scoped, its current value comes from the control-plane
// database and the user writes it through the control-plane API, it reads empty
// before any write without failing, and the written weekly period is what the
// settings projection reads afterwards (SETTINGS-AC-65, SETTINGS-TC-136).
package settingstc136

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/tsic404/kos-settings/pkg/kossettings"
	"github.com/tsic404/kos-settings/tests/acceptance/fixtures"
	"github.com/tsic404/kosd/pkg/settings"
)

// window is one weekly period, the value shape SETTINGS-C-72 settles on.
const window = "Sun 02:00-04:00"

// TestTC136MaintenanceWindowItemReadWrite reads the window item before any
// write, writes one weekly period, and reads the item again through both the
// settings domain and the settings page projection.
func TestTC136MaintenanceWindowItemReadWrite(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	values := fixtures.NewValues()
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, values)

	// The product contributes one cluster-scoped item the user writes; the
	// entry names the control-plane write that changes it (C-36, C-72).
	item := settingOf(t, shell, "", kossettings.ClusterMaintenanceWindow)
	if item.Scope != "Cluster" {
		t.Fatalf("window scope = %q, want Cluster", item.Scope)
	}
	if item.Writer != "User" {
		t.Fatalf("window writer = %q, want User", item.Writer)
	}
	if !item.Editable {
		t.Fatal("the window item is not editable, so the user cannot write it")
	}
	if item.Entry == nil {
		t.Fatal("the window item carries no modification entry")
	}
	if item.Entry.Method != http.MethodPut || item.Entry.Path != "/api/v1/settings/"+kossettings.ClusterMaintenanceWindow {
		t.Fatalf("window entry = %+v, want the settings write", item.Entry)
	}

	// Nothing is written yet: the value is empty and reading it is not an
	// error, which is the unset policy SETTINGS-C-72 leaves unrestricted.
	if item.Value != "" {
		t.Fatalf("unwritten window value = %q, want empty", item.Value)
	}
	pageItem := fixtures.ItemOf(t, fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings), kossettings.ClusterMaintenanceWindow)
	if pageItem.Source != "database" {
		t.Fatalf("window page source = %q, want database", pageItem.Source)
	}
	if value, _ := pageItem.Value.(string); value != "" {
		t.Fatalf("unwritten window page value = %q, want empty", pageItem.Value)
	}
	if pageItem.Entry == nil || pageItem.Entry.Path != item.Entry.Path {
		t.Fatalf("window page entry = %+v, want %q", pageItem.Entry, item.Entry.Path)
	}

	// The user writes a weekly period the way the entry names: a PUT on the
	// declared control-plane path through the settings domain.
	if _, failure := shell.Settings.Set(context.Background(), fixtures.Owner, kossettings.ClusterMaintenanceWindow, "", window); failure != nil {
		t.Fatalf("write the maintenance window: %s (%s)", failure.Message, failure.Code)
	}

	// The same item now reads the written period from the database.
	item = settingOf(t, shell, "", kossettings.ClusterMaintenanceWindow)
	if item.Value != window {
		t.Fatalf("window value = %q, want %q", item.Value, window)
	}
	pageItem = fixtures.ItemOf(t, fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings), kossettings.ClusterMaintenanceWindow)
	if value, _ := pageItem.Value.(string); value != window {
		t.Fatalf("window page value = %q, want %q", pageItem.Value, window)
	}
}

// settingOf reads one item of the settings domain projection.
func settingOf(t *testing.T, shell *fixtures.Shell, node, id string) settings.Item {
	t.Helper()
	items, failure := shell.Settings.Items(context.Background(), fixtures.Owner, node)
	if failure != nil {
		t.Fatalf("read the settings domain: %s (%s)", failure.Message, failure.Code)
	}
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("settings item %q is absent: %+v", id, items)
	return settings.Item{}
}
