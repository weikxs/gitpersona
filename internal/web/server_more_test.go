package web

import (
	"context"
	"net/http"
	"testing"
	"time"
)

const serveTimeout = 5 * time.Second

func TestNewRejectsAnAddressWithoutAPort(t *testing.T) {
	if _, err := New("127.0.0.1", Handlers{}); err == nil {
		t.Fatal("New() accepted an address without a port")
	}
}

func TestARequestWithoutASessionCookieIsNotAValidSession(t *testing.T) {
	s := newTestServer(t)

	if s.validSession(request("GET", "/api/ping", nil)) {
		t.Fatal("validSession() = true for a request without the cookie")
	}
}

func TestACookieWithTheWrongValueIsNotAValidSession(t *testing.T) {
	s := newTestServer(t)
	login(t, s)
	r := request("GET", "/api/ping", nil)
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: "not-the-session"})

	if s.validSession(r) {
		t.Fatal("validSession() = true for a cookie that is not a session")
	}
}

func TestServeStopsWhenItsContextEnds(t *testing.T) {
	ln, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ln.Addr().String(), Handlers{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, ln) }()

	cancel()

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve() = %v, want a clean stop", err)
		}
	case <-time.After(serveTimeout):
		t.Fatal("Serve() did not stop when its context ended")
	}
}

func TestServeReportsAListenerThatIsAlreadyClosed(t *testing.T) {
	ln, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ln.Addr().String(), Handlers{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)

	go func() { served <- s.Serve(context.Background(), ln) }()

	select {
	case err := <-served:
		if err == nil {
			t.Fatal("Serve() on a closed listener returned nil, want the listener error")
		}
	case <-time.After(serveTimeout):
		t.Fatal("Serve() on a closed listener did not return")
	}
}
