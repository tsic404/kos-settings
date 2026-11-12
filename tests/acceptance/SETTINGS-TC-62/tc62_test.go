// Package settingstc62 verifies that installing the kos-settings extension
// makes the settings page appear with exactly the cluster and node items
// SETTINGS-C-36 declares, each carrying one readable source type, and that the
// modifiable policy item has a modification entry while the node-level
// configuration states why it has none (SETTINGS-AC-31, SETTINGS-TC-62).
package settingstc62

import (
	"bytes"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tsic404/kos-sdk/packaging"
	"github.com/tsic404/kosd/pkg/identity"

	"github.com/tsic404/kos-settings/pkg/kossettings"
	"github.com/tsic404/kos-settings/tests/acceptance/fixtures"
)

func TestTC62ClusterAndNodeSettingsItems(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Node(t, cluster, "node-a", "4", "8Gi", true, false, map[string]string{"node-role.kubernetes.io/worker": ""})
	fixtures.Node(t, cluster, "node-b", "2", "4Gi", true, true, nil)
	fixtures.Provider(t, cluster, "standard.example",
		fixtures.Capability{Name: "storage.block", Version: "1", Scope: "node", State: "Verified"})

	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues())

	// The installed component's page is listed, so the panel can open it
	// (C-22). The page disappears with the component, which the uninstall case
	// judges.
	var listed bool
	for _, summary := range shell.Pages.List(fixtures.Owner) {
		if summary.ID == kossettings.PageSettings {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("the settings page is not listed for the installed product: %+v", shell.Pages.List(fixtures.Owner))
	}

	page := fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings)
	if len(page.Items) != len(fixtures.SettingsPageItems) {
		t.Fatalf("settings page has %d items, want the %d SETTINGS-C-36 declares: %+v",
			len(page.Items), len(fixtures.SettingsPageItems), page.Items)
	}

	// Each declared item carries exactly one source type and the entry the
	// contract gives it: a fact has a reason and no entry, the policy item has
	// an entry and no reason (C-36, C-53).
	seen := map[string]bool{}
	for _, expected := range fixtures.SettingsPageItems {
		item := fixtures.ItemOf(t, page, expected.ID)
		if seen[item.ID] {
			t.Fatalf("item %q is rendered twice", item.ID)
		}
		seen[item.ID] = true
		if item.Source != expected.Source {
			t.Fatalf("item %q source = %q, want %q (C-36)", item.ID, item.Source, expected.Source)
		}
		if item.Value == nil {
			t.Fatalf("item %q carries no current value", item.ID)
		}
		switch {
		case expected.Modifiable && item.Entry == nil:
			t.Fatalf("modifiable item %q has no modification entry", item.ID)
		case expected.Modifiable && item.Reason != "":
			t.Fatalf("modifiable item %q carries the refusal reason %q", item.ID, item.Reason)
		case !expected.Modifiable && item.Entry != nil:
			t.Fatalf("item %q offers the modification entry %+v it must not have", item.ID, item.Entry)
		case !expected.Modifiable && item.Reason == "":
			t.Fatalf("item %q has no entry and no reason", item.ID)
		}
	}

	// The node-level configuration outside the object model is the item
	// SETTINGS-C-36 names as unmodifiable: it states why and offers no entry.
	config := fixtures.ItemOf(t, page, kossettings.NodeLevelConfiguration)
	if config.Reason != kossettings.NodeConfigurationReason {
		t.Fatalf("node-level configuration reason = %q, want the declared reason", config.Reason)
	}

	// The modifiable item's entry names the control-plane write that changes it
	// (C-23).
	window := fixtures.ItemOf(t, page, kossettings.ClusterMaintenanceWindow)
	if window.Entry == nil || window.Entry.Path != "/api/v1/settings/"+kossettings.ClusterMaintenanceWindow {
		t.Fatalf("cluster policy entry = %+v, want the settings write", window.Entry)
	}

	// A signed-in account without the Owner role reads the page but receives no
	// modification entry (C-59).
	member := fixtures.ReadPage(t, shell, fixtures.Member, kossettings.PageSettings)
	if entry := fixtures.ItemOf(t, member, kossettings.ClusterMaintenanceWindow).Entry; entry != nil {
		t.Fatalf("the member reads the modification entry %+v, want none (C-59)", entry)
	}

	// The usage item has no metric source in this distribution: it is still
	// readable and states that instead of presenting capacity as consumption
	// (C-71).
	usage, ok := fixtures.ItemOf(t, page, kossettings.ClusterUsage).Value.(map[string]any)
	if !ok || usage["available"] != false || usage["reason"] == "" {
		t.Fatalf("cluster usage value = %v, want an unavailable item with a reason", usage)
	}
}

// TestTC62OnlyOwnerSeesTheMaintenanceEntries checks that the page's write
// actions are the Owner's alone, so the entries do not appear to another role
// (KOSD-C-59, SETTINGS-C-36).
func TestTC62OnlyOwnerSeesTheMaintenanceEntries(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Node(t, cluster, "node-a", "4", "8Gi", true, false, nil)
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues())

	owner := fixtures.ReadPage(t, shell, fixtures.Owner, kossettings.PageSettings)
	actions := map[string]bool{}
	for _, action := range owner.Actions {
		actions[action.ID] = true
	}
	for _, id := range []string{kossettings.ActionStartMaintenance, kossettings.ActionRestoreMaintenance, kossettings.ActionSetMaintenanceWindow} {
		if !actions[id] {
			t.Fatalf("the owner's page is missing action %q: %+v", id, owner.Actions)
		}
	}

	for _, session := range []identity.Session{fixtures.Member} {
		page := fixtures.ReadPage(t, shell, session, kossettings.PageSettings)
		for _, action := range page.Actions {
			t.Fatalf("role %q receives the action %q, want none", session.Role, action.ID)
		}
	}
}

// TestTC62SettingsPageAssetsShipInThePackage checks the settings page's browser
// resources enter the package under assets/<path> and that the digest covers
// them: the artifact verifies against the digest of the content it carries, and
// a changed asset breaks that promise so install and upgrade are refused
// (KOSD-C-17, SDK-C-4).
func TestTC62SettingsPageAssetsShipInThePackage(t *testing.T) {
	source := filepath.Join("..", "..", "..", "assets")
	onDisk := filesUnder(t, source)
	if _, entry := onDisk[packaging.AssetPrefix+"settings/index.html"]; !entry {
		t.Fatalf("the settings page ships no entry document: %v", slices.Sorted(maps.Keys(onDisk)))
	}

	assets, err := kossettings.Assets(source)
	if err != nil {
		t.Fatalf("read the settings page assets: %v", err)
	}
	if len(assets) != len(onDisk) {
		t.Fatalf("loader read %d assets, want the %d files under %q", len(assets), len(onDisk), source)
	}
	for _, asset := range assets {
		content, shipped := onDisk[asset.Path]
		if !shipped {
			t.Fatalf("loader read %q, which is not a file under %q", asset.Path, source)
		}
		if !bytes.Equal(asset.Content, content) {
			t.Fatalf("loader read different bytes for %q", asset.Path)
		}
	}

	built, archive, err := kossettings.Build(assets)
	if err != nil {
		t.Fatalf("build the package with the settings page assets: %v", err)
	}
	verified, err := packaging.Verify(archive, built.Digest)
	if err != nil {
		t.Fatalf("verify the built package: %v", err)
	}
	if len(verified.Assets) != len(onDisk) {
		t.Fatalf("package carries %d assets, want the %d the product ships", len(verified.Assets), len(onDisk))
	}
	for index, asset := range assets {
		if !strings.HasPrefix(asset.Path, packaging.AssetPrefix) {
			t.Fatalf("asset %q is outside the %q namespace", asset.Path, packaging.AssetPrefix)
		}
		packaged := verified.Assets[index]
		if packaged.Path != asset.Path || !bytes.Equal(packaged.Content, onDisk[asset.Path]) {
			t.Fatalf("packaged asset %d = %q, want %q with the file's bytes", index, packaged.Path, asset.Path)
		}
	}

	// One changed asset changes the digest, and the archive built before the
	// change no longer satisfies it (C-17).
	changed := append([]packaging.Asset(nil), assets...)
	changed[0].Content = append(append([]byte(nil), assets[0].Content...), '\n')
	updated, _, err := kossettings.Build(changed)
	if err != nil {
		t.Fatalf("build the package with a changed asset: %v", err)
	}
	if updated.Digest == built.Digest {
		t.Fatal("changing a settings page asset did not change the package digest")
	}
	if _, err := packaging.Verify(archive, updated.Digest); err == nil {
		t.Fatal("the unchanged package verified against the digest its changed asset no longer matches")
	}
}

// filesUnder reads every file under root, keyed by the package path it ships
// as. It is the independent record the loader and the package are judged
// against, so a file the loader drops cannot pass unnoticed.
func filesUnder(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[packaging.AssetPrefix+filepath.ToSlash(relative)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("read %q: %v", root, err)
	}
	return files
}
