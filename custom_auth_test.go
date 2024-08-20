package auth_token_exchange_plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/mgladysheva/auth-token-exchange-plugin"
)

func TestCustomAuth(t *testing.T) {
	cfg := traefik_custom_auth.CreateConfig()
	cfg.AuthURL = "https://example.com/verify"
	cfg.Production = false

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := traefik_custom_auth.New(ctx, next, cfg, "custom-auth-plugin")
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