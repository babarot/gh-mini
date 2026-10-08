// Command vendorjs fetches the Mermaid and MathJax files the pages load from
// npm into the vendor directory given, at the versions below, checking each
// package against its integrity. To update one, change its version and
// integrity, as npm view <name>@<version> dist.integrity prints it, run
//
//	go run ./hack/vendorjs internal/server/assets/vendor
//
// and read the diff.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type pkg struct {
	name, version, integrity string
	// dest is where a file of the package goes in the vendor directory,
	// or "" to leave it out. Names are as in the tarball, under package/.
	dest func(name string) string
	// dirs are the directories the package fills alone, emptied first so
	// that files dropped upstream go
	dirs []string
}

var pkgs = []pkg{
	{
		name:      "mermaid",
		version:   "11.13.0",
		integrity: "sha512-fEnci+Immw6lKMFI8sqzjlATTyjLkRa6axrEgLV2yHTfv8r+h1wjFbV6xeRtd4rUV1cS4EpR9rwp3Rci7TRWDw==",
		dest: func(name string) string {
			switch name {
			case "dist/mermaid.min.js":
				return "mermaid.min.js"
			case "LICENSE":
				return "mermaid.license"
			}
			return ""
		},
	},
	{
		name:      "mathjax",
		version:   "4.1.3",
		integrity: "sha512-BN/8Pkgn7G1pIDYJqd9md+JHsE/jydSYbyOZnSdSA0WziuVO8mRxdYiWFumkVVly/8U+hm9DpIIoWuvySverzw==",
		dest: func(name string) string {
			switch {
			case name == "tex-mml-chtml.js", name == "sre/speech-worker.js",
				strings.HasPrefix(name, "input/tex/extensions/"):
				return "mathjax/" + name
			case strings.HasPrefix(name, "sre/mathmaps/"):
				// The English and braille maps, the only locale the page
				// asks for
				switch path.Base(name) {
				case "base.json", "en.json", "euro.json", "nemeth.json":
					return "mathjax/" + name
				}
			case name == "LICENSE":
				return "mathjax.license"
			}
			return ""
		},
		dirs: []string{"mathjax"},
	},
	{
		name:      "@mathjax/mathjax-newcm-font",
		version:   "4.1.3",
		integrity: "sha512-gzAB3dFHilHX1l5x2xUqRL+1jDQt3Fyza1DkEMVXWC4E8SvsGdlgEza47HYi2WhVcgfkvf4zgUGzuhbq3Pjlew==",
		dest: func(name string) string {
			if strings.HasPrefix(name, "chtml/") {
				return "mathjax-newcm-font/" + name
			}
			return ""
		},
		dirs: []string{"mathjax-newcm-font"},
	},
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		log.Fatal("usage: vendorjs VENDOR_DIR")
	}
	vendor := os.Args[1]
	for _, p := range pkgs {
		if err := fetch(p, vendor); err != nil {
			log.Fatalf("%s@%s: %v", p.name, p.version, err)
		}
	}
	// The font's package names its license without a file of it
	note := "mathjax-newcm-font/ holds the chtml directory of @mathjax/mathjax-newcm-font\n" +
		pkgs[2].version + " from npm, which its package.json licenses under the Apache License,\n" +
		"Version 2.0. The text of the license is in mathjax.license.\n"
	if err := os.WriteFile(filepath.Join(vendor, "mathjax-newcm-font.license"), []byte(note), 0o644); err != nil {
		log.Fatal(err)
	}
}

func fetch(p pkg, vendor string) error {
	url := fmt.Sprintf("https://registry.npmjs.org/%s/-/%s-%s.tgz", p.name, path.Base(p.name), p.version)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	sum := sha512.Sum512(b)
	if got := "sha512-" + base64.StdEncoding.EncodeToString(sum[:]); got != p.integrity {
		return fmt.Errorf("integrity %s, want %s", got, p.integrity)
	}
	for _, d := range p.dirs {
		if err := os.RemoveAll(filepath.Join(vendor, d)); err != nil {
			return err
		}
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, ok := strings.CutPrefix(h.Name, "package/")
		if !ok || h.Typeflag != tar.TypeReg || strings.Contains(name, "..") {
			continue
		}
		dest := p.dest(name)
		if dest == "" {
			continue
		}
		out := filepath.Join(vendor, filepath.FromSlash(dest))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return err
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("no files taken from %s", url)
	}
	log.Printf("%s@%s: %d files", p.name, p.version, n)
	return nil
}
