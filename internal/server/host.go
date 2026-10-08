package server

import (
	"net"
	"net/http"
	"strings"
)

// A page on another site can reach this server by DNS rebinding: its own
// name, made to resolve to 127.0.0.1, makes the server its origin, and so
// every file readable to its scripts. The browser still sends that name
// as Host, so the server answers only names it is known by: localhost, an
// IP address, which no one can rebind, and the names in Options.Hosts.

// knownHost tells whether a request's Host is one the server is known by.
func (s *Server) knownHost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	switch {
	case host == "localhost", strings.HasSuffix(host, ".localhost"):
		return true
	case net.ParseIP(host) != nil:
		return true
	}
	for _, h := range s.opts.Hosts {
		if strings.EqualFold(strings.TrimSuffix(h, "."), host) {
			return true
		}
	}
	return false
}

// checkHost refuses requests by a name the server is not known by.
func (s *Server) checkHost(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.knownHost(r) {
			http.Error(w, "gh-mini: unknown host "+r.Host+"; open it at localhost or by an IP address", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}
