package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func sayBye(t *testing.T, s *Server, cookie *http.Cookie) {
	t.Helper()
	r := request("POST", "/api/bye", map[string]string{"Origin": "http://" + addr, csrfHeader: s.CSRFToken()})
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("bye: status %d", w.Code)
	}
}

func TestTheServerStopsSoonAfterThePageCloses(t *testing.T) {
	s := newTestServer(t)
	s.byeGrace = 20 * time.Millisecond
	done := s.idleTimer()
	cookie := login(t, s)

	sayBye(t, s, cookie)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the server kept running after the page said goodbye")
	}
}

func TestAReloadKeepsTheServerRunning(t *testing.T) {
	s := newTestServer(t)
	s.byeGrace = 100 * time.Millisecond
	done := s.idleTimer()
	cookie := login(t, s)

	sayBye(t, s, cookie)
	// The reloaded page asks for itself right away.
	r := request("GET", "/", nil)
	r.AddCookie(cookie)
	s.ServeHTTP(httptest.NewRecorder(), r)

	select {
	case <-done:
		t.Fatal("the server stopped although the page came back")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestByeNeedsTheCSRFToken(t *testing.T) {
	s := newTestServer(t)
	s.byeGrace = 10 * time.Millisecond
	done := s.idleTimer()
	cookie := login(t, s)
	r := request("POST", "/api/bye", map[string]string{"Origin": "http://" + addr})
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("bye without the token: status %d, want 403", w.Code)
	}
	select {
	case <-done:
		t.Fatal("a forged goodbye stopped the server")
	case <-time.After(100 * time.Millisecond):
	}
}
