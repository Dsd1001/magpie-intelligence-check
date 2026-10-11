package check

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTicketExchangeAndExpiry(t *testing.T) {
	a := NewAccess("control", "http://localhost")
	ticket := a.Ticket()
	session, ok := a.Exchange(ticket)
	if !ok || !a.Session(session) {
		t.Fatal("ticket failed")
	}
	if _, ok = a.Exchange(ticket); ok {
		t.Fatal("ticket reused")
	}
	old := a.Ticket()
	a.mu.Lock()
	a.tickets[old] = time.Now().Add(-time.Second)
	a.mu.Unlock()
	if _, ok = a.Exchange(old); ok {
		t.Fatal("expired ticket accepted")
	}
	a.SetRemote("https://fixture.trycloudflare.com")
	if a.Session(session) {
		t.Fatal("old session survived remote reset")
	}
}
func TestConsoleTicketHTTP(t *testing.T) {
	s := fixture(t)
	a := NewAccess("control", "http://example.com")
	h := s.ManagedHandler(a, nil)
	ticket := a.Ticket()
	r := httptest.NewRequest("GET", "http://example.com/open/"+ticket, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("no session")
	}
	r = httptest.NewRequest("GET", "http://example.com/api/state", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("GET", "http://example.com/open/"+ticket, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("ticket reused")
	}
	r = httptest.NewRequest("POST", "http://example.com/api/console", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("browser can sign tickets")
	}
}
