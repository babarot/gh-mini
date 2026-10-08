package server

import (
	"net/http"
	"testing"
)

func TestAboutDialog(t *testing.T) {
	root, themes := newTestRepo(t)
	for _, tt := range []struct {
		name         string
		opts         Options
		want, reject []string
	}{
		{
			name: "revision",
			opts: Options{Version: "1.2.3 (abc1234)", Revision: "abc1234def"},
			want: []string{
				`<dialog class="settings about" id="mini.about"`,
				`<a href="https://github.com/babarot/gh-mini/commit/abc1234def" target="_blank" rel="noopener">1.2.3 (abc1234)</a>`,
				`<div class="menu-version">1.2.3 (abc1234)</div>`,
				`data-open="mini.shortcuts"`,
				`<dialog class="settings shortcuts" id="mini.shortcuts"`,
			},
		},
		{
			name:   "no revision",
			opts:   Options{Version: "1.2.3"},
			want:   []string{`<dd>1.2.3</dd>`},
			reject: []string{`/commit/`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Root, tt.opts.Name, tt.opts.ThemesDir = root, "repo", themes
			srv, err := New(tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { srv.Close() })
			r := get(t, srv.Handler(), "/")
			r.expect(t, http.StatusOK, append(tt.want, `<dd>`+srv.serving+`</dd>`)...)
			r.reject(t, tt.reject...)
		})
	}
}

func TestShortenHome(t *testing.T) {
	for _, tt := range []struct{ root, home, want string }{
		{"/home/user/notes", "/home/user", "~/notes"},
		{"/home/user/a/b", "/home/user", "~/a/b"},
		{"/home/user", "/home/user", "~"},
		{"/home/userx/notes", "/home/user", "/home/userx/notes"},
		{"/srv/notes", "/home/user", "/srv/notes"},
		{"/srv/notes", "", "/srv/notes"},
		{"/srv/notes", "/", "/srv/notes"},
	} {
		if got := shortenHome(tt.root, tt.home); got != tt.want {
			t.Errorf("shortenHome(%q, %q) = %q, want %q", tt.root, tt.home, got, tt.want)
		}
	}
}
