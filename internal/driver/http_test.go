package driver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rakrisi/flowtest/internal/config"
	"github.com/rakrisi/flowtest/internal/engine"
)

func TestHTTPDriver_Execution_HeadersAndEmptyResponse(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty" {
			w.Header().Add("X-Test-Single", "single-val")
			w.Header().Add("X-Test-Multi", "val1")
			w.Header().Add("X-Test-Multi", "val2")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	d := &HTTPDriver{}

	t.Run("normal JSON response", func(t *testing.T) {
		cfg := &config.APIConfig{
			Method: "GET",
			URL:    server.URL + "/normal",
		}
		res, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resp := res["response"].(map[string]interface{})
		if resp["status"].(int) != http.StatusOK {
			t.Errorf("status = %v, want 200", resp["status"])
		}

		body := resp["body"].(map[string]interface{})
		if body["status"] != "ok" {
			t.Errorf("body.status = %q, want 'ok'", body["status"])
		}
	})

	t.Run("empty response handles properly", func(t *testing.T) {
		cfg := &config.APIConfig{
			Method: "GET",
			URL:    server.URL + "/empty",
		}
		res, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resp := res["response"].(map[string]interface{})
		if resp["status"].(int) != http.StatusNoContent {
			t.Errorf("status = %v, want 204", resp["status"])
		}

		if resp["body"] != nil {
			t.Errorf("body = %v, want nil", resp["body"])
		}

		// Verify joined headers
		headers := resp["headers"].(map[string]interface{})
		if headers["X-Test-Single"] != "single-val" {
			t.Errorf("headers[X-Test-Single] = %v, want 'single-val'", headers["X-Test-Single"])
		}
		if headers["X-Test-Multi"] != "val1, val2" {
			t.Errorf("headers[X-Test-Multi] = %v, want 'val1, val2'", headers["X-Test-Multi"])
		}

		// Verify raw headers
		rawHeaders := resp["raw_headers"].(map[string]interface{})
		multiVals := rawHeaders["X-Test-Multi"].([]interface{})
		if len(multiVals) != 2 || multiVals[0] != "val1" || multiVals[1] != "val2" {
			t.Errorf("raw_headers[X-Test-Multi] = %v, want ['val1', 'val2']", rawHeaders["X-Test-Multi"])
		}
	})
}
