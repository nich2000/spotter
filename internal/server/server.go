package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"spotter/internal/app"
	"spotter/internal/sse"
)

type Server struct {
	logger *slog.Logger
	app    *app.App
	broker *sse.Broker
	static http.Handler
}

func New(logger *slog.Logger, app *app.App, broker *sse.Broker) *Server {
	return &Server{
		logger: logger,
		app:    app,
		broker: broker,
		static: http.FileServer(http.Dir("./web")),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.broker.Handler)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.Handle("/", s.static)
	return logRequests(s.logger, localOnly(mux))
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.app.State())
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.app.Refresh(r.Context()))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.RemoteAddr
		if r.Header.Get("X-Forwarded-For") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if host == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("http request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
