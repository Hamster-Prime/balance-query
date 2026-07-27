// Package providers contains balance fetchers for each supported AI platform.
package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// getJSON performs GET with Bearer auth and decodes JSON into dest.
func getJSON(url, bearerToken string, dest any) error {
	return doGet(url, "Bearer "+bearerToken, dest)
}

// getJSONRawAuth performs GET with a raw Authorization value (no "Bearer " prefix).
// Used by GLM whose API requires the token directly.
func getJSONRawAuth(url, rawToken string, dest any) error {
	return doGet(url, rawToken, dest)
}

func doGet(url, authHeader string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func errResult(authID, provider, msg string) balance.Result {
	return balance.Result{
		Provider:  provider,
		AuthID:    authID,
		Error:     msg,
		FetchedAt: time.Now(),
	}
}
