package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownHTTPDrainsRequests(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "finished")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	srv.Config.RegisterOnShutdown(func() { close(release) })
	client := srv.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := shutdownHTTP(ctx, srv.Config); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "finished" {
		t.Fatalf("in-flight response was interrupted: body=%q error=%v", body, err)
	}
}

func TestShutdownHTTPClosesExpiredStreams(t *testing.T) {
	finished, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	client := srv.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := shutdownHTTP(ctx, srv.Config); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("expired shutdown left the stream and its request context open")
	}
}
