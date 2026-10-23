package server

import (
	"log/slog"
	"net/http"
)

// Routes registers a group of routes on the router.
type Routes interface {
	Register(mux *http.ServeMux)
}

// NewRouter registers every route group and wraps them with the common
// middleware.
func NewRouter(log *slog.Logger, groups ...Routes) http.Handler {
	mux := http.NewServeMux()
	for _, g := range groups {
		g.Register(mux)
	}

	// Logging is outermost so recovered panics are logged with their 500 status.
	return logRequests(log, recoverPanics(log, mux))
}
