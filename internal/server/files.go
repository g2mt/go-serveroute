package server

import (
	"net/http"
)

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request, path string) {
	fs := http.FileServer(http.Dir(path))
	http.StripPrefix("/", fs).ServeHTTP(w, r)
}
