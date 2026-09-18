package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newCSRFTestTransport points a Transport at a test server.
func newCSRFTestTransport(t *testing.T, srv *httptest.Server) *Transport {
	t.Helper()
	cfg := NewConfig(srv.URL, "user", "pass")
	tr := NewTransport(cfg)
	return tr
}

// TestFetchCSRFTokenHeadFastPath: when HEAD answers with a token, that is the only
// request made — the fast path must not cost a second round trip.
func TestFetchCSRFTokenHeadFastPath(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			w.Header().Set("X-CSRF-Token", "TOKEN-FROM-HEAD")
			w.WriteHeader(http.StatusOK)
		default:
			gets.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-HEAD" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("expected one HEAD and no GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// TestFetchCSRFTokenGetFallback reproduces BASIS 740 / ECC EhP7: HEAD is answered
// with 400 and no token, and only GET yields one. Without the fallback vsp cannot
// talk to those systems at all.
func TestFetchCSRFTokenGetFallback(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		default:
			gets.Add(1)
			w.Header().Set("X-CSRF-Token", "TOKEN-FROM-GET")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-GET" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("expected one HEAD then one GET, got %d/%d", heads.Load(), gets.Load())
	}
}

func TestTransportRequestCSRF403ForcesGETRefresh(t *testing.T) {
	var discoveryPaths []string
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case csrfCoreDiscoveryPath, csrfCompatibilityGraphPath:
			discoveryPaths = append(discoveryPaths, r.URL.Path)
			if r.Method == http.MethodHead {
				w.Header().Set("X-CSRF-Token", "HEAD-TOKEN")
			} else {
				w.Header().Set("X-CSRF-Token", "GET-TOKEN")
			}
			w.WriteHeader(http.StatusOK)
		case "/sap/bc/adt/write":
			if writes.Add(1) == 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if got := r.Header.Get("X-CSRF-Token"); got != "GET-TOKEN" {
				t.Errorf("POST token = %q, want GET-TOKEN", got)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if _, err := tr.Request(context.Background(), "/sap/bc/adt/write", &RequestOptions{Method: http.MethodPost}); err != nil {
		t.Fatalf("POST after forced GET refresh: %v", err)
	}
	want := strings.Join([]string{csrfCoreDiscoveryPath, csrfCompatibilityGraphPath}, ",")
	if got := strings.Join(discoveryPaths, ","); got != want {
		t.Fatalf("CSRF paths = %q, want %q", got, want)
	}
}

func TestTransportRequestCSRFGetFallbackDiscardsFailedHeadSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == csrfCoreDiscoveryPath && r.Method == http.MethodHead:
			http.SetCookie(w, &http.Cookie{Name: "SAP_SESSIONID", Value: "from-head", Path: "/"})
			w.Header().Set("X-CSRF-Token", "INVALID-HEAD-TOKEN")
			w.WriteHeader(http.StatusBadRequest)
		case r.URL.Path == csrfCoreDiscoveryPath && r.Method == http.MethodGet:
			if cookie, err := r.Cookie("SAP_SESSIONID"); err == nil {
				t.Errorf("GET inherited failed HEAD session cookie %q", cookie.Value)
			}
			http.SetCookie(w, &http.Cookie{Name: "SAP_SESSIONID", Value: "from-get", Path: "/"})
			w.Header().Set("X-CSRF-Token", "GET-TOKEN")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/sap/bc/adt/write":
			cookie, err := r.Cookie("SAP_SESSIONID")
			if err != nil || cookie.Value != "from-get" {
				t.Errorf("POST session cookie = %v, want from-get", cookie)
			}
			if got := r.Header.Get("X-CSRF-Token"); got != "GET-TOKEN" {
				t.Errorf("POST token = %q, want GET-TOKEN", got)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if _, err := tr.Request(context.Background(), "/sap/bc/adt/write", &RequestOptions{Method: http.MethodPost}); err != nil {
		t.Fatalf("POST after fresh GET fallback: %v", err)
	}
}

// A 401 must be reported as an authentication failure without a pointless retry.
func TestFetchCSRFTokenUnauthorizedDoesNotRetry(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	err := tr.fetchCSRFToken(context.Background())
	if err == nil {
		t.Fatal("expected an error for 401")
	}
	if requests.Load() != 1 {
		t.Fatalf("401 must not be retried, made %d requests", requests.Load())
	}
}

// The "Required" placeholder is not a token; it must trigger the fallback.
func TestFetchCSRFTokenRequiredPlaceholderFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("X-CSRF-Token", "Required")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("X-CSRF-Token", "REAL-TOKEN")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "REAL-TOKEN" {
		t.Fatalf("token = %q", got)
	}
}

// A 403 on HEAD is not a verdict on the user's authorizations: some systems
// refuse the HEAD and answer the GET perfectly well. Short-circuiting there
// reintroduced exactly the unusability the GET fallback exists to prevent.
func TestFetchCSRFTokenForbiddenHeadStillTriesGet(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		gets.Add(1)
		w.Header().Set("X-CSRF-Token", "TOKEN-FROM-GET")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("a 403 on HEAD must not stop the GET fallback: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-GET" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("expected one HEAD then one GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// A 403 from both methods is a real authorization failure and must say so.
func TestFetchCSRFTokenForbiddenEverywhereFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	err := newCSRFTestTransport(t, srv).fetchCSRFToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected a 403 authorization error, got %v", err)
	}
}
