package icsmcp

import (
	"context"
	"io"
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

func TestCalendarRefreshAndValidationApplyFeedURLGuards(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.allowPrivateCalendarHosts = false
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "local", Name: "Local", URL: "http://127.0.0.1/calendar.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	if err := svc.RefreshCalendar(ctx, cal.ID, svc.now()); err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("RefreshCalendar() error = %v, want private-host rejection", err)
	}
	result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: cal.URL})
	if err == nil || result.OK || !strings.Contains(result.Error, "private or local") {
		t.Fatalf("ValidateCalendar() result=%#v error=%v, want private-host rejection", result, err)
	}
}

func TestCalendarRefreshAndValidationApplyResponseSizeLimit(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.maxCalendarBytes = 3
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("four"))}, nil
	})}
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/calendar.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	if err := svc.RefreshCalendar(ctx, cal.ID, svc.now()); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("RefreshCalendar() error = %v, want response-size rejection", err)
	}
	result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: cal.URL})
	if err == nil || result.OK || !strings.Contains(result.Error, "byte limit") {
		t.Fatalf("ValidateCalendar() result=%#v error=%v, want response-size rejection", result, err)
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
