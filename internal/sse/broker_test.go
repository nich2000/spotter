package sse

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamingLifecycle(t *testing.T) {
	b := NewBroker(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e := b.Publish(map[string]int{"revision": 1}); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { b.Handler(w, r); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		b.mu.Lock()
		count := len(b.clients)
		b.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 20; i++ {
		if e := b.Publish(map[string]int{"revision": i}); e != nil {
			t.Fatal(e)
		}
	}
	cancel()
	<-done
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatal("empty stream")
	}
	if e := b.Publish(make(chan int)); e == nil {
		t.Fatal("unsupported JSON")
	}
	if e := b.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := b.Close(ctx); e == nil {
		t.Fatal("cancelled")
	}
	plain := &noFlush{header: http.Header{}}
	b.Handler(plain, httptest.NewRequest("GET", "/events", nil))
	if plain.status != 500 {
		t.Fatal(plain.status)
	}
}

type noFlush struct {
	header http.Header
	status int
}

func (w *noFlush) Header() http.Header         { return w.header }
func (w *noFlush) Write(b []byte) (int, error) { return len(b), nil }
func (w *noFlush) WriteHeader(n int)           { w.status = n }
