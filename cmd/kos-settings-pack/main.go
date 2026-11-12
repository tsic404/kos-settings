// Command kos-settings-pack builds the kos-settings extension package with the
// author SDK: manifest, UI assets and the digest that covers both (KOSD-C-17,
// SDK-C-4). The same tool packs a third-party extension; nothing here reads a
// product identity.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsic404/kos-settings/pkg/kossettings"
)

func main() {
	var (
		assetsDir = flag.String("assets", "assets", "directory of UI assets, laid out as assets/<path> in the package")
		out       = flag.String("out", filepath.Join("dist", "kos-settings.tar"), "package archive to write")
		digestOut = flag.String("digest-out", "", "file to write the package digest to (default: <out>.digest)")
	)
	flag.Parse()
	if err := run(*assetsDir, *out, *digestOut); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(assetsDir, out, digestOut string) error {
	assets, err := kossettings.Assets(assetsDir)
	if err != nil {
		return err
	}
	pkg, archive, err := kossettings.Build(assets)
	if err != nil {
		return err
	}
	if digestOut == "" {
		digestOut = out + ".digest"
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(out, archive, 0o644); err != nil {
		return fmt.Errorf("write package: %w", err)
	}
	if err := os.WriteFile(digestOut, []byte(pkg.Digest.String()+"\n"), 0o644); err != nil {
		return fmt.Errorf("write digest: %w", err)
	}
	fmt.Println(pkg.Digest)
	return nil
}
