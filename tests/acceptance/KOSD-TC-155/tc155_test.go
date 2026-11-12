// Package kosdtc155 verifies that kos-settings ships as an independently
// installable extension package: the artifact is the author SDK's structure
// with a digest over the declaration, the kernel activates exactly the declared
// pages and settings, uninstalling the component withdraws only them, and a
// reinstall or a second product leaves the kernel object model and the other
// product untouched (KOSD-AC-72, KOSD-TC-155).
package kosdtc155

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/tsic404/kos-sdk/packaging"
	"github.com/tsic404/kosd/pkg/extension"

	"github.com/tsic404/kos-settings/pkg/kossettings"
	"github.com/tsic404/kos-settings/tests/acceptance/fixtures"
)

func TestTC155PackageStructure(t *testing.T) {
	pkg, archive, err := kossettings.Build(nil)
	if err != nil {
		t.Fatalf("build the extension package: %v", err)
	}
	verified, err := packaging.Verify(archive, pkg.Digest)
	if err != nil {
		t.Fatalf("verify the built package: %v", err)
	}
	if len(verified.Assets) != 0 {
		t.Fatalf("the package carries assets the product did not declare: %+v", verified.Assets)
	}

	// The declaration's closed field set is the one KOSD-C-18 fixes; nothing is
	// added and nothing is left out (SDK-C-3).
	var declared map[string]any
	encoded, err := json.Marshal(verified.Manifest)
	if err != nil {
		t.Fatalf("encode the packaged manifest: %v", err)
	}
	if err := json.Unmarshal(encoded, &declared); err != nil {
		t.Fatalf("decode the packaged manifest: %v", err)
	}
	fields := make([]string, 0, len(declared))
	for field := range declared {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	want := []string{"config", "contributions", "dependencies", "frameworkVersion", "id", "permissions", "status", "uninstallRetention", "version"}
	if len(fields) != len(want) {
		t.Fatalf("manifest fields = %v, want the closed set %v", fields, want)
	}
	for index, field := range want {
		if fields[index] != field {
			t.Fatalf("manifest fields = %v, want %v", fields, want)
		}
	}

	// The declaration the kernel activates and the declaration the package
	// ships are the same contributions: one table produces both, so the visible
	// items cannot drift from the packaged ones (C-17, C-18).
	activated := fixtures.DeclarationMap(t)
	if declared["id"] != activated["identifier"] {
		t.Fatalf("package id = %v, want %v", declared["id"], activated["identifier"])
	}
	if declared["version"] != activated["version"] || declared["frameworkVersion"] != activated["frameworkVersion"] {
		t.Fatalf("package version pair = %v/%v, want %v/%v",
			declared["version"], declared["frameworkVersion"], activated["version"], activated["frameworkVersion"])
	}
	shipped, ok := declared["contributions"].(map[string]any)
	if !ok {
		t.Fatalf("packaged contributions are not an object: %v", declared["contributions"])
	}
	loaded, ok := activated["contributions"].(map[string]any)
	if !ok {
		t.Fatalf("activated contributions are not an object: %v", activated["contributions"])
	}
	for _, class := range []string{"pages", "settings"} {
		if _, present := shipped[class]; !present {
			t.Fatalf("the package ships no %q contribution: %v", class, shipped)
		}
		if !sameJSON(t, shipped[class], loaded[class]) {
			t.Fatalf("packaged %q contribution = %v, want the activated one %v", class, shipped[class], loaded[class])
		}
	}
}

func TestTC155DigestCoversAssets(t *testing.T) {
	plain, _, err := kossettings.Build(nil)
	if err != nil {
		t.Fatalf("build the bare package: %v", err)
	}
	first, _, err := kossettings.Build([]packaging.Asset{{Path: packaging.AssetPrefix + "settings.js", Content: []byte("v1")}})
	if err != nil {
		t.Fatalf("build the package with one asset: %v", err)
	}
	second, _, err := kossettings.Build([]packaging.Asset{{Path: packaging.AssetPrefix + "settings.js", Content: []byte("v2")}})
	if err != nil {
		t.Fatalf("build the package with a changed asset: %v", err)
	}
	if plain.Digest == first.Digest {
		t.Fatal("adding a UI asset did not change the package digest")
	}
	if first.Digest == second.Digest {
		t.Fatal("changing a UI asset did not change the package digest")
	}
}

func TestTC155LoadUnloadReload(t *testing.T) {
	created := time.Unix(1, 0)
	settings := fixtures.Product(t, "settings", created)
	other := fixtures.OtherProduct(t, created.Add(time.Second))

	both := fixtures.Install(t, settings, other)
	assertContribution(t, both, kossettings.PageSettings, true)
	assertContribution(t, both, "other-page", true)
	assertSetting(t, both, kossettings.ClusterMaintenanceWindow, true)
	assertSetting(t, both, "other-setting", true)

	// Uninstall kos-settings alone: its page and setting disappear, the other
	// product keeps both, and the kernel model is untouched (C-29).
	after := fixtures.Install(t, other)
	assertContribution(t, after, kossettings.PageSettings, false)
	assertSetting(t, after, kossettings.ClusterMaintenanceWindow, false)
	assertContribution(t, after, "other-page", true)
	assertSetting(t, after, "other-setting", true)
	if classes := after.ObjectClasses(); len(classes) != 0 {
		t.Fatalf("uninstalling a page-only product left object classes: %+v", classes)
	}
	if len(after.Capabilities()) != len(both.Capabilities()) {
		t.Fatalf("uninstalling changed the kernel capability baseline: %d, want %d",
			len(after.Capabilities()), len(both.Capabilities()))
	}

	// Reinstall the same package: the same contributions come back (KOSD-AC-72).
	reloaded := fixtures.Install(t, settings, other)
	assertContribution(t, reloaded, kossettings.PageSettings, true)
	assertSetting(t, reloaded, kossettings.ClusterMaintenanceWindow, true)

	// A package whose artifact does not match the named digest installs
	// nothing: the page and setting do not appear (C-17).
	broken := settings
	broken.Package = &extension.Package{
		Reference: settings.Package.Reference,
		Digest:    "sha256:" + zeros(),
		Content:   settings.Package.Content,
	}
	if registry := fixtures.Install(t, broken); len(registry.Pages()) != 0 || len(registry.Settings()) != 0 {
		t.Fatalf("a digest mismatch contributed a page or setting: %+v %+v", registry.Pages(), registry.Settings())
	}
}

// TestTC155FirstPartyAndThirdPartyShareThePackageStructure checks that the
// package the product ships has the same structure a third-party extension gets
// from the same SDK path (SDK-C-4).
func TestTC155FirstPartyAndThirdPartyShareThePackageStructure(t *testing.T) {
	_, firstParty, err := kossettings.Build(nil)
	if err != nil {
		t.Fatalf("build the product package: %v", err)
	}
	_, thirdParty, err := packaging.Pack(map[string]any{
		"id": "third-party-demo", "version": "1.0.0", "frameworkVersion": ">=1.0 <2.0",
		"contributions":      map[string]any{"pages": []any{map[string]any{"id": "demo"}}},
		"dependencies":       map[string]any{},
		"permissions":        map[string]any{},
		"config":             map[string]any{},
		"status":             map[string]any{},
		"uninstallRetention": map[string]any{},
	}, nil)
	if err != nil {
		t.Fatalf("pack the third-party declaration: %v", err)
	}
	firstEntries := entriesOf(t, firstParty)
	thirdEntries := entriesOf(t, thirdParty)
	if len(firstEntries) != len(thirdEntries) {
		t.Fatalf("first-party entries = %v, third-party entries = %v", firstEntries, thirdEntries)
	}
	for index := range firstEntries {
		if firstEntries[index] != thirdEntries[index] {
			t.Fatalf("first-party entries = %v, third-party entries = %v", firstEntries, thirdEntries)
		}
	}
}

func assertContribution(t *testing.T, registry *extension.Registry, id string, want bool) {
	t.Helper()
	found := false
	for _, page := range registry.Pages() {
		if page.ID == id {
			found = true
		}
	}
	if found != want {
		t.Fatalf("page %q present = %t, want %t", id, found, want)
	}
}

func assertSetting(t *testing.T, registry *extension.Registry, id string, want bool) {
	t.Helper()
	if _, found := registry.Setting(id); found != want {
		t.Fatalf("setting %q present = %t, want %t", id, found, want)
	}
}

// entriesOf lists the archive entries in order.
func entriesOf(t *testing.T, archive []byte) []string {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("open the package archive: %v", err)
	}
	defer reader.Close()
	var entries []string
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read the package archive: %v", err)
		}
		entries = append(entries, header.Name)
	}
	return entries
}

// sameJSON compares two decoded JSON values by their canonical encoding.
func sameJSON(t *testing.T, left, right any) bool {
	t.Helper()
	leftBytes, err := json.Marshal(left)
	if err != nil {
		t.Fatalf("encode the packaged value: %v", err)
	}
	rightBytes, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("encode the activated value: %v", err)
	}
	return bytes.Equal(leftBytes, rightBytes)
}

func zeros() string {
	const digits = "0000000000000000000000000000000000000000000000000000000000000000"
	return digits
}
