package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ──────────────────────────────────────────────
// MLflow API client
// ──────────────────────────────────────────────

// validateCredentials checks if the given username/password are valid
// against the MLflow server and returns whether the user is an admin.
func validateCredentials(username, password string) (bool, error) {
	reqURL := fmt.Sprintf("%s/api/2.0/mlflow/users/get?username=%s", mlflowURL, url.QueryEscape(username))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(username, password)

	client := &http.Client{Timeout: 10 * time.Second}
	req.Host = "localhost:5000"
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to reach MLflow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return false, fmt.Errorf("invalid credentials")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("MLflow returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		User struct {
			IsAdmin bool `json:"is_admin"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.User.IsAdmin, nil
}

// mlflowRequest is a generic helper to make authenticated requests
// to the MLflow API. Returns the HTTP status code, response body, and error.
func mlflowRequest(method, path string, body interface{}, username, password string) (int, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	reqURL := mlflowURL + path
	req, err := http.NewRequest(method, reqURL, reqBody)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(username, password)
	req.Host = "localhost:5000"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, respBody, nil
}
