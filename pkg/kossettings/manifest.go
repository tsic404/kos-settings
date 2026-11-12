// Package kossettings is the kos-settings product: the cluster and node
// settings the panel shows (KOSD-C-36) and the packaged declaration that
// installs them (KOSD-C-17, KOSD-C-18).
package kossettings

import (
	"encoding/json"
	"fmt"
)

// Manifest returns the declaration in the closed field set of KOSD-C-18 that
// the author SDK validates and packs (SDK-C-3): identifier, version, target
// framework range, contributions and the blocks the contract requires even
// when this product declares nothing in them.
//
// The contributions are rendered from the same Go declaration the kernel
// activates, so the manifest the package ships and the declaration the kernel
// loads cannot drift.
func Manifest() (map[string]any, error) {
	declaration := Declaration()
	contributions := map[string]any{}
	if len(declaration.Contributions.Pages) > 0 {
		pages, err := jsonValue(declaration.Contributions.Pages)
		if err != nil {
			return nil, err
		}
		contributions["pages"] = pages
	}
	if len(declaration.Contributions.Settings) > 0 {
		settings, err := jsonValue(declaration.Contributions.Settings)
		if err != nil {
			return nil, err
		}
		contributions["settings"] = settings
	}
	return map[string]any{
		"id":               declaration.Identifier,
		"version":          declaration.Version,
		"frameworkVersion": declaration.FrameworkVersion,
		"contributions":    contributions,
		"dependencies":     map[string]any{},
		"permissions":      map[string]any{},
		"config":           map[string]any{},
		"status":           map[string]any{},
		"uninstallRetention": map[string]any{
			// The written values survive an uninstall and the page does not:
			// KOSD-C-23 keeps a component's written values under the declared
			// retention, and a page holds no data of its own.
			"contributions": map[string]any{
				"pages":    "delete",
				"settings": "retain",
			},
		},
	}, nil
}

// jsonValue renders one part of the declaration as the JSON the manifest
// carries.
func jsonValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode declaration: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, fmt.Errorf("decode declaration: %w", err)
	}
	return decoded, nil
}
