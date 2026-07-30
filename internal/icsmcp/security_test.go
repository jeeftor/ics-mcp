package icsmcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCalendarRequestRejectsPrivateHostsByDefault(t *testing.T) {
	svc := newTestService(t)
	svc.allowPrivateCalendarHosts = false
	if _, err := svc.calendarRequest(context.Background(), "http://127.0.0.1/feed.ics"); err == nil {
		t.Fatal("calendarRequest() accepted a loopback URL")
	}
	if _, err := svc.calendarRequest(context.Background(), "file:///tmp/feed.ics"); err == nil {
		t.Fatal("calendarRequest() accepted a non-HTTP URL")
	}
}

func TestReadCalendarBodyEnforcesLimit(t *testing.T) {
	svc := newTestService(t)
	svc.maxCalendarBytes = 3
	if _, err := svc.readCalendarBody(strings.NewReader("four")); err == nil {
		t.Fatal("readCalendarBody() accepted an oversized response")
	}
}

func TestBearerTokenProtectsAPIAndMCP(t *testing.T) {
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandlerWithOptions(svc, NewMCPServer(svc), HTTPOptions{BearerToken: "secret"}))
	defer server.Close()

	response, err := http.Get(server.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", response.StatusCode)
	}
}
