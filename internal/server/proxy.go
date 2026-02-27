package server

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, target string) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	url, err := url.Parse(target)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		req.Header.Set("Host", "localhost")
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Real-IP", r.RemoteAddr)

		if r.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}

	proxy.ServeHTTP(w, r)
}
