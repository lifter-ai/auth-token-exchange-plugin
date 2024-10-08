package auth_token_exchange_plugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	plugin "github.com/lifter-ai/auth-token-exchange-plugin"
)

func TestCustomAuth(t *testing.T) {
	// Mock auth server
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"id": "123"}); err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer authServer.Close()

	cfg := plugin.CreateConfig()
	cfg.AuthURL = authServer.URL
	cfg.Production = true

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Check X-User-Id header
		if userID := req.Header.Get("X-User-Id"); userID != "123" {
			t.Errorf("Expected X-User-Id to be '123', got '%s'", userID)
		}

		// Check X-Request-Id header
		if requestID := req.Header.Get("X-Request-Id"); requestID == "" {
			t.Error("Expected X-Request-Id to be set")
		} else {
			uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
			if !uuidRegex.MatchString(requestID) {
				t.Errorf("X-Request-Id is not a valid UUID v7: %v", requestID)
			}
		}

		// Check that Authorization header was removed
		if auth := req.Header.Get("Authorization"); auth != "" {
			t.Errorf("Expected Authorization header to be removed, got '%s'", auth)
		}

		rw.WriteHeader(http.StatusOK)
	})

	handler, err := plugin.New(ctx, next, cfg, "custom-auth-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Invalid status code: got %v want %v", recorder.Code, http.StatusOK)
	}
}
