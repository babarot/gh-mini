package server

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestPageCSP(t *testing.T) {
	srv, _ := newPreviewServer(t)
	for _, target := range []string{"/", "/README.md", "/nope"} {
		csp := get(t, srv.Handler(), target).header.Get("Content-Security-Policy")
		for _, want := range []string{"script-src 'self';", "base-uri 'none'", "object-src 'none'", "form-action 'none'", "frame-src http://localhost:7000;"} {
			if !strings.Contains(csp+";", want) {
				t.Errorf("%s: CSP %q lacks %q", target, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: CSP allows eval", target)
		}
	}
	plain := newServerFor(t, Options{})
	if csp := get(t, plain.Handler(), "/").header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'none'") {
		t.Errorf("without previews: %q", csp)
	}
}

// With script-src 'self', an inline script would not run: every script in
// the page must come from a file, but for JSON data.
func TestPageHasNoInlineScripts(t *testing.T) {
	b, err := assets.ReadFile("assets/page.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range regexp.MustCompile(`<script[^>]*>`).FindAllString(string(b), -1) {
		if !strings.Contains(tag, " src=") && !strings.Contains(tag, `type="application/json"`) {
			t.Errorf("inline script: %s", tag)
		}
	}
	if r := get(t, newTestServer(t), "/", sameOrigin); r.code != http.StatusOK {
		t.Fatal(r.code)
	}
}
