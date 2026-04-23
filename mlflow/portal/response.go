package main

import (
	"encoding/json"
	"net/http"
)

// ──────────────────────────────────────────────
// JSON response helpers
// ──────────────────────────────────────────────

// jsonResponse writes a JSON-encoded response with 200 OK status.
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON-encoded error response with the given status code.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// proxyRaw forwards a raw response body with the given status code.
func proxyRaw(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(body)
}
