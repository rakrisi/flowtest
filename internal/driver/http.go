package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/radhe-singh/flowtest/internal/config"
	"github.com/radhe-singh/flowtest/internal/engine"
)

// HTTPDriver executes HTTP API calls.
type HTTPDriver struct{}

func (d *HTTPDriver) Name() string { return "http" }

func (d *HTTPDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	api, ok := stepConfig.(*config.APIConfig)
	if !ok {
		return nil, fmt.Errorf("http driver: invalid step config type %T", stepConfig)
	}

	// Build full URL
	reqURL := api.URL
	if !strings.HasPrefix(reqURL, "http://") && !strings.HasPrefix(reqURL, "https://") {
		if env.APIBase == "" {
			return nil, fmt.Errorf("http driver: url %q is relative but env.api_base is not set", reqURL)
		}
		reqURL = strings.TrimRight(env.APIBase, "/") + "/" + strings.TrimLeft(reqURL, "/")
	}

	// Handle API key in query param
	if api.Auth != nil && api.Auth.APIKey != nil && api.Auth.APIKey.Query != "" {
		parsedURL, err := url.Parse(reqURL)
		if err != nil {
			return nil, fmt.Errorf("http driver: parsing URL for api_key query: %w", err)
		}
		q := parsedURL.Query()
		q.Set(api.Auth.APIKey.Query, api.Auth.APIKey.Value)
		parsedURL.RawQuery = q.Encode()
		reqURL = parsedURL.String()
	}

	// Encode body
	var bodyReader io.Reader
	if api.Body != nil {
		bodyBytes, err := json.Marshal(api.Body)
		if err != nil {
			return nil, fmt.Errorf("http driver: encoding body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	timeout := 30 * time.Second
	if api.Timeout > 0 {
		timeout = api.Timeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, strings.ToUpper(api.Method), reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http driver: creating request: %w", err)
	}

	// Set headers
	if api.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range api.Headers {
		req.Header.Set(k, v)
	}

	// Apply authentication
	if err := applyAuth(req, api.Auth); err != nil {
		return nil, fmt.Errorf("http driver: applying auth: %w", err)
	}

	// Execute
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http driver: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body (capped at 10MB to prevent OOM from large responses)
	const maxResponseSize = 10 << 20 // 10MB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("http driver: reading response: %w", err)
	}

	// Build result
	result := map[string]interface{}{
		"response": map[string]interface{}{
			"status":  resp.StatusCode,
			"headers": headerMap(resp.Header),
		},
	}

	// Try to parse as JSON
	var parsed interface{}
	if err := json.Unmarshal(respBody, &parsed); err == nil {
		result["response"].(map[string]interface{})["body"] = parsed
	} else {
		result["response"].(map[string]interface{})["body"] = string(respBody)
	}

	return result, nil
}

// applyAuth applies authentication to the HTTP request based on config.
func applyAuth(req *http.Request, auth *config.AuthConfig) error {
	if auth == nil {
		return nil
	}

	// Bearer token authentication
	if auth.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+auth.Bearer)
		return nil
	}

	// Basic authentication
	if auth.Basic != nil {
		credentials := auth.Basic.Username + ":" + auth.Basic.Password
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
		req.Header.Set("Authorization", "Basic "+encoded)
		return nil
	}

	// API Key authentication (header variant)
	if auth.APIKey != nil && auth.APIKey.Header != "" {
		req.Header.Set(auth.APIKey.Header, auth.APIKey.Value)
		return nil
	}

	// API Key in query param is handled earlier in URL construction
	return nil
}

func headerMap(h http.Header) map[string]interface{} {
	m := make(map[string]interface{}, len(h))
	for k, v := range h {
		if len(v) == 1 {
			m[k] = v[0]
		} else {
			vals := make([]interface{}, len(v))
			for i, s := range v {
				vals[i] = s
			}
			m[k] = vals
		}
	}
	return m
}
