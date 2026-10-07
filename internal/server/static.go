package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// staticFiles serves the server's own files under /_mini/static/<version>/,
// where the version is a hash of their contents. A URL then always names
// the same bytes, so browsers can keep them for good. The version is in the
// path rather than the query so that what a file loads by a relative URL,
// such as an ES module's imports, gets the same version.
type staticFiles struct {
	version string
	assets  http.Handler
	chroma  string
}

func newStaticFiles() *staticFiles {
	assetsFS, _ := fs.Sub(assets, "assets")
	chroma := chromaCSS()

	h := sha256.New()
	_ = fs.WalkDir(assetsFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(assetsFS, p)
		if err != nil {
			return nil
		}
		io.WriteString(h, p)
		h.Write(b)
		return nil
	})
	io.WriteString(h, chroma)

	return &staticFiles{
		version: hex.EncodeToString(h.Sum(nil))[:12],
		assets:  http.FileServer(http.FS(assetsFS)),
		chroma:  chroma,
	}
}

// prefix is the URL the files are under.
func (st *staticFiles) prefix() string {
	return "/_mini/static/" + st.version
}

func (st *staticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/_mini/static/")
	version, rest, _ := strings.Cut(rest, "/")
	// A page from an older build may still ask for its version; it gets
	// the current files, kept only until they change
	if version == st.version {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	kind, file, _ := strings.Cut(rest, "/")
	switch {
	case kind == "assets":
		serveSub(st.assets, w, r, file)
	case rest == "chroma.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		io.WriteString(w, st.chroma)
	default:
		http.NotFound(w, r)
	}
}

func serveSub(h http.Handler, w http.ResponseWriter, r *http.Request, file string) {
	r2 := new(http.Request)
	*r2 = *r
	u := *r.URL
	u.Path = "/" + file
	u.RawPath = ""
	r2.URL = &u
	h.ServeHTTP(w, r2)
}
