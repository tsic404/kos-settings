package kossettings

import (
	"fmt"

	"github.com/tsic404/kos-sdk/packaging"
)

// Build packs the product into the extension artifact KOSD-C-17 distributes:
// the declaration, the UI assets supplied by the caller, and the digest that
// covers both (SDK-C-4). It is the one packing path every author uses; the
// packer reads no product identity, so a first-party system application and a
// third-party extension produce the same structure.
//
// A declaration the SDK rejects yields an error and no archive.
func Build(assets []packaging.Asset) (packaging.Package, []byte, error) {
	manifest, err := Manifest()
	if err != nil {
		return packaging.Package{}, nil, err
	}
	pkg, archive, err := packaging.Pack(manifest, assets)
	if err != nil {
		return packaging.Package{}, nil, fmt.Errorf("pack kos-settings: %w", err)
	}
	return pkg, archive, nil
}
