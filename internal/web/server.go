// Package web serves the local configuration page. It only listens on the
// loopback interface and protects every request: a one-time key in the
// launch URL becomes a session cookie, the Host header must be the loopback
// address (against DNS rebinding), changes need a matching Origin and a CSRF
// token, and a strict Content Security Policy allows only the page's own
// files. The server stops after a period without requests.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed static
var static embed.FS

const (
	sessionCookie   = "gitpersona_session" // plus the port: see cookieName
	csrfHeader      = "X-GitPersona-CSRF"
	tokenBytes      = 32
	defaultIdle     = 15 * time.Minute
	readTimeout     = 10 * time.Second
	csrfPlaceholder = "{{CSRF}}"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// Handlers are the API routes, as ServeMux patterns ("GET /api/state").
type Handlers struct {
	API map[string]http.HandlerFunc
}

// Server is the configuration page server.
type Server struct {
	addr string
	key  string
	csrf string
	mux  *http.ServeMux
	idle time.Duration

	mu       sync.Mutex
	keyUsed  bool
	sessions []string
	timer    *time.Timer
	done     chan struct{}
	stopOnce sync.Once
}

// New prepares a server for addr, which must be a loopback address.
func New(addr string, h Handlers) (*Server, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("refusing to listen on %s: only loopback addresses are allowed", addr)
	}
	s := &Server{addr: addr, key: randomToken(), csrf: randomToken(), mux: http.NewServeMux(),
		idle: defaultIdle, done: make(chan struct{})}
	for pattern, handler := range h.API {
		s.mux.HandleFunc(pattern, handler)
	}
	s.mux.HandleFunc("GET /{$}", s.index)
	staticFS, _ := fs.Sub(static, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))
	return s, nil
}

// Register adds API routes after the server exists, so routes such as
// "close" can use the server's Stop.
func (s *Server) Register(routes map[string]http.HandlerFunc) {
	for pattern, handler := range routes {
		s.mux.HandleFunc(pattern, handler)
	}
}

// Key is the one-time key for the launch URL.
func (s *Server) Key() string { return s.key }

// CSRFToken is the token changes must carry in the X-GitPersona-CSRF header.
func (s *Server) CSRFToken() string { return s.csrf }

// URL is the launch URL to open in a browser.
func (s *Server) URL() string { return "http://" + s.addr + "/?key=" + s.key }

// ServeHTTP enforces every protection before routing.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())
	if r.Host != s.addr {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.touch()
	if r.URL.Query().Has("key") {
		s.exchangeKey(w, r)
		return
	}
	if !s.validSession(r) {
		http.Error(w, "forbidden: this page needs the one-time link GitPersona printed in your terminal; "+
			"each link works once, so run `gitpersona ui` again for a new one", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.validChange(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) exchangeKey(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keyUsed || !equal(r.URL.Query().Get("key"), s.key) {
		http.Error(w, "forbidden: this link was already used; run `gitpersona ui` again", http.StatusForbidden)
		return
	}
	s.keyUsed = true
	session := randomToken()
	s.sessions = append(s.sessions, session)
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName())
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		if equal(c.Value, session) {
			return true
		}
	}
	return false
}

func (s *Server) validChange(r *http.Request) bool {
	return r.Header.Get("Origin") == "http://"+s.addr && equal(r.Header.Get(csrfHeader), s.csrf)
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	page, err := static.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "missing page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(strings.Replace(string(page), csrfPlaceholder, s.csrf, 1)))
}

// Listen opens a listener on a random loopback port; pass its address to New.
func Listen() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

// Serve serves on ln until ctx ends, Stop is called, or the server has been idle.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s, ReadHeaderTimeout: readTimeout}
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
	case <-s.idleTimer():
	case err := <-errs:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Stop ends Run, for example after the page asks to close.
func (s *Server) Stop() { s.stopOnce.Do(func() { close(s.done) }) }

// idleTimer starts the idle clock and returns the channel closed when the
// server has had no request for the idle period.
func (s *Server) idleTimer() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer == nil {
		s.timer = time.AfterFunc(s.idle, s.Stop)
	}
	return s.done
}

func (s *Server) touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Reset(s.idle)
	}
}

func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
}

func randomToken() string {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error()) // cannot happen on supported systems
	}
	return hex.EncodeToString(b)
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// cookieName includes the port: browsers share cookies between ports of the
// same host, so a second page must not replace the first page's session.
func (s *Server) cookieName() string {
	_, port, _ := net.SplitHostPort(s.addr)
	return sessionCookie + "_" + port
}
