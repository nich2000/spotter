package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

type Broker struct {
	logger  *slog.Logger
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	latest  []byte
}

func NewBroker(logger *slog.Logger) *Broker {
	return &Broker{
		logger:  logger,
		clients: make(map[chan []byte]struct{}),
	}
}

func (b *Broker) Publish(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal sse payload: %w", err)
	}

	b.mu.Lock()
	b.latest = payload
	clients := make([]chan []byte, 0, len(b.clients))
	for client := range b.clients {
		clients = append(clients, client)
	}
	b.mu.Unlock()

	for _, client := range clients {
		select {
		case client <- payload:
		default:
		}
	}
	return nil
}

func (b *Broker) Handler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 8)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	latest := append([]byte(nil), b.latest...)
	count := len(b.clients)
	b.mu.Unlock()
	b.logger.Info("sse client connected", "clients", count)

	defer func() {
		b.mu.Lock()
		delete(b.clients, ch)
		count := len(b.clients)
		b.mu.Unlock()
		close(ch)
		b.logger.Info("sse client disconnected", "clients", count)
	}()

	if len(latest) > 0 {
		writeEvent(w, latest)
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-ch:
			writeEvent(w, payload)
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, payload []byte) {
	fmt.Fprintf(w, "event: update\n")
	fmt.Fprintf(w, "data: %s\n\n", payload)
}

func (b *Broker) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
