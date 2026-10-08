package server

import (
	"net/http"
	"testing"
)

func TestUnknownHost(t *testing.T) {
	srv, _ := newPreviewServer(t)
	srv.opts.Hosts = []string{"box.local"}
	token := withCookie("gh-mini-preview-7000", srv.previewToken)
	for host, known := range map[string]bool{
		"localhost":           true,
		"localhost:6419":      true,
		"LOCALHOST.:6419":     true,
		"docs.localhost:6419": true,
		"127.0.0.1:6419":      true,
		"[::1]:6419":          true,
		"192.168.1.20:6419":   true,
		"box.local:6419":      true,
		"Box.Local":           true,
		"attacker.example":    false,
		"localhost.example":   false,
		"box.local.example":   false,
		"":                    false,
	} {
		want := http.StatusForbidden
		if known {
			want = http.StatusOK
		}
		setHost := func(r *http.Request) { r.Host = host }
		for _, tt := range []struct {
			name string
			code int
		}{
			{"tree", get(t, srv.Handler(), "/_mini/api/tree", setHost).code},
			{"raw", get(t, srv.Handler(), "/README.md?raw", setHost).code},
			{"preview", previewGet(t, srv, http.MethodGet, "/site/index.html", token, setHost).code},
		} {
			if tt.code != want {
				t.Errorf("%s with Host %q: status = %d, want %d", tt.name, host, tt.code, want)
			}
		}
	}
}
