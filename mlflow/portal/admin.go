package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

// ──────────────────────────────────────────────
// Admin middleware
// ──────────────────────────────────────────────

// adminContext holds the authenticated admin user's claims and password
// for proxying requests to MLflow on their behalf.
type adminContext struct {
	claims   *Claims
	password string
}

// getAdminContext validates that the request is from an authenticated admin.
// Returns the admin context and true if valid, or writes an error and returns false.
func getAdminContext(w http.ResponseWriter, r *http.Request) (*adminContext, bool) {
	claims, err := getClaimsFromRequest(r)
	if err != nil {
		jsonError(w, "Not authenticated", http.StatusUnauthorized)
		return nil, false
	}
	if !claims.IsAdmin {
		jsonError(w, "Admin access required", http.StatusForbidden)
		return nil, false
	}
	password := getPasswordFromToken(r)
	if password == "" {
		jsonError(w, "Session expired, please login again", http.StatusUnauthorized)
		return nil, false
	}
	return &adminContext{claims: claims, password: password}, true
}

// ──────────────────────────────────────────────
// Admin API: User Management
// ──────────────────────────────────────────────

func handleAdminGetUser(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		jsonError(w, "username query parameter required", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("GET",
		"/api/2.0/mlflow/users/get?username="+url.QueryEscape(username),
		nil, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("POST",
		"/api/2.0/mlflow/users/create",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("DELETE",
		"/api/2.0/mlflow/users/delete",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminUpdatePassword(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("PATCH",
		"/api/2.0/mlflow/users/update-password",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminUpdateAdmin(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("PATCH",
		"/api/2.0/mlflow/users/update-admin",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

// ──────────────────────────────────────────────
// Admin API: Experiment Permissions
// ──────────────────────────────────────────────

func handleAdminCreateExpPerm(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		ExperimentID string `json:"experiment_id"`
		Username     string `json:"username"`
		Permission   string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("POST",
		"/api/2.0/mlflow/experiments/permissions/create",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminUpdateExpPerm(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		ExperimentID string `json:"experiment_id"`
		Username     string `json:"username"`
		Permission   string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("PATCH",
		"/api/2.0/mlflow/experiments/permissions/update",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminDeleteExpPerm(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		ExperimentID string `json:"experiment_id"`
		Username     string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("DELETE",
		"/api/2.0/mlflow/experiments/permissions/delete",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

// ──────────────────────────────────────────────
// Admin API: Registered Model Permissions
// ──────────────────────────────────────────────

func handleAdminCreateModelPerm(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Name       string `json:"name"`
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("POST",
		"/api/2.0/mlflow/registered-models/permissions/create",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminUpdateModelPerm(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Name       string `json:"name"`
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("PATCH",
		"/api/2.0/mlflow/registered-models/permissions/update",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminDeleteModelPerm(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status, body, err := mlflowRequest("DELETE",
		"/api/2.0/mlflow/registered-models/permissions/delete",
		req, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

// ──────────────────────────────────────────────
// Admin API: List Resources
// ──────────────────────────────────────────────

func handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	// Query PostgreSQL auth DB for all usernames
	db, err := sql.Open("postgres", authDBURI)
	if err != nil {
		jsonError(w, "Failed to connect to auth DB: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT username FROM users ORDER BY id")
	if err != nil {
		jsonError(w, "Failed to query users: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var uname string
		if err := rows.Scan(&uname); err == nil {
			usernames = append(usernames, uname)
		}
	}

	// Fetch full user details (with permissions) from MLflow API in parallel
	type userResult struct {
		Data json.RawMessage
		Err  error
	}
	results := make([]userResult, len(usernames))
	var wg sync.WaitGroup

	for i, uname := range usernames {
		wg.Add(1)
		go func(idx int, username string) {
			defer wg.Done()
			status, body, err := mlflowRequest("GET",
				"/api/2.0/mlflow/users/get?username="+url.QueryEscape(username),
				nil, ctx.claims.Username, ctx.password)
			if err != nil || status != 200 {
				results[idx] = userResult{Err: fmt.Errorf("failed to get user %s", username)}
				return
			}
			var resp struct {
				User json.RawMessage `json:"user"`
			}
			if err := json.Unmarshal(body, &resp); err == nil {
				results[idx] = userResult{Data: resp.User}
			}
		}(i, uname)
	}
	wg.Wait()

	var users []json.RawMessage
	for _, r := range results {
		if r.Data != nil {
			users = append(users, r.Data)
		}
	}

	jsonResponse(w, map[string]interface{}{"users": users})
}

func handleAdminListExperiments(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	status, body, err := mlflowRequest("GET",
		"/api/2.0/mlflow/experiments/search?max_results=1000",
		nil, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}

func handleAdminListModels(w http.ResponseWriter, r *http.Request) {
	ctx, ok := getAdminContext(w, r)
	if !ok {
		return
	}

	status, body, err := mlflowRequest("GET",
		"/api/2.0/mlflow/registered-models/search?max_results=100",
		nil, ctx.claims.Username, ctx.password)
	if err != nil {
		jsonError(w, "Failed to reach MLflow: "+err.Error(), http.StatusBadGateway)
		return
	}
	proxyRaw(w, status, body)
}
