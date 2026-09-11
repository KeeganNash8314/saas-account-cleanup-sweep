package infrai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestCreateCronRetriesRateLimitWithSameWriteIdentity(t *testing.T) {
	var calls int
	var firstKey string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.URL.Path != createCronPath {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer authorization")
		}
		key := req.Header.Get("Idempotency-Key")
		if calls == 1 {
			firstKey = key
			return response(http.StatusTooManyRequests, `{"ok":false,"data":null,"error":{"message":"try later"},"metadata":{}}`, "0"), nil
		}
		if key == "" || key != firstKey {
			t.Fatal("retry changed write identity")
		}
		return response(http.StatusOK, `{"ok":true,"data":{"job_id":"job-42"},"error":null,"metadata":{}}`, ""), nil
	})
	client := NewCronClient("test-key", &http.Client{Transport: transport})
	client.sleep = func(context.Context, time.Duration) error { return nil }

	jobID, err := client.CreateCron(context.Background(), "15 2 * * *", "https://tenant.example/admin/sweep")
	if err != nil || jobID != "job-42" || calls != 2 {
		t.Fatalf("job=%q calls=%d err=%v", jobID, calls, err)
	}
}

func TestCreateCronSurfacesEnvelopeBeforeHTTPStatus(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusBadRequest, `{"ok":false,"data":null,"error":{"message":"schedule rejected"},"metadata":{}}`, ""), nil
	})
	client := NewCronClient("test-key", &http.Client{Transport: transport})

	_, err := client.CreateCron(context.Background(), "bad", "https://tenant.example/admin/sweep")
	apiErr, ok := err.(*InfraiError)
	if !ok || apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "schedule rejected" {
		t.Fatalf("expected envelope error, got %v", err)
	}
}

func response(status int, body, retryAfter string) *http.Response {
	header := make(http.Header)
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}
