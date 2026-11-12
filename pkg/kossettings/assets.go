package kossettings

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tsic404/kos-sdk/packaging"
)

// Assets reads every file under dir as one package asset, keyed by its
// package-relative path under assets/. The package digest covers each entry
// with its name and length, so renaming or editing a UI asset changes the
// digest an install request is bound to (C-17, SDK-C-4).
//
// A missing directory is an empty asset set: the settings page renders from the
// control-plane projection, so the product stays installable without UI assets.
func Assets(dir string) ([]packaging.Asset, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read assets directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("assets path %q is not a directory", dir)
	}
	var assets []packaging.Asset
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read asset %q: %w", path, err)
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		assets = append(assets, packaging.Asset{
			Path:    packaging.AssetPrefix + filepath.ToSlash(relative),
			Content: content,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(assets, func(left, right int) bool {
		return strings.Compare(assets[left].Path, assets[right].Path) < 0
	})
	return assets, nil
}
