package avatar

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpdateUserAvatarIncludesUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["user_id"] != "user-7" {
			t.Fatalf("user_id = %#v", body["user_id"])
		}
		if len(body) != 2 {
			t.Fatalf("body = %#v", body)
		}
		_, _ = io.WriteString(w, `{"ok":true,"data":{},"error":null,"metadata":{}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", server.Client())
	if err := client.UpdateUserAvatar(context.Background(), "user-7", "image-1", "operation:release"); err != nil {
		t.Fatalf("UpdateUserAvatar: %v", err)
	}
}

func TestClientDecodesEnvelopeBeforeHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"ok":false,"data":null,"error":{"code":"test","message":"policy rejected input"},"metadata":{}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", server.Client())
	_, err := client.Upload(context.Background(), []byte("image"), "avatar.png", "operation:upload")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.HTTPStatus != http.StatusUnprocessableEntity || apiErr.Code != "test" {
		t.Fatalf("error = %#v", err)
	}
}

func TestClientRetries429WithSameIdempotencyKey(t *testing.T) {
	var calls int
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"ok":false,"data":null,"error":{"code":"test","message":"retry later"},"metadata":{}}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"data":{"id":"image-1"},"error":null,"metadata":{}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", server.Client())
	client.sleep = func(context.Context, time.Duration) error { return nil }
	image, err := client.Upload(context.Background(), []byte("image"), "avatar.png", "operation:upload")
	if err != nil || image != "image-1" {
		t.Fatalf("image = %q, error = %v", image, err)
	}
	if calls != 2 || strings.Join(keys, ",") != "operation:upload,operation:upload" {
		t.Fatalf("calls = %d, keys = %v", calls, keys)
	}
}
