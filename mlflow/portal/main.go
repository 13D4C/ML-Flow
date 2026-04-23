package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
)

// Config is loaded from environment variables via config.go.
// See .env.example for the full list of available variables.

// ──────────────────────────────────────────────
// JWT helpers
// ──────────────────────────────────────────────

type Claims struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func createToken(username string, isAdmin bool) (string, error) {
	claims := Claims{
		Username: username,
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func parseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// ──────────────────────────────────────────────
// Credential helpers
// ──────────────────────────────────────────────

func getClaimsFromRequest(r *http.Request) (*Claims, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, err
	}
	return parseToken(cookie.Value)
}

func getPasswordFromToken(r *http.Request) string {
	c, err := r.Cookie("mlflow_creds")
	if err != nil {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	return string(decoded)
}

func setCredentialsCookie(w http.ResponseWriter, password string) {
	encoded := base64.StdEncoding.EncodeToString([]byte(password))
	http.SetCookie(w, &http.Cookie{
		Name:     "mlflow_creds",
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

// ──────────────────────────────────────────────
// MLflow credential validation
// ──────────────────────────────────────────────

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

// ──────────────────────────────────────────────
// MLflow API proxy helper
// ──────────────────────────────────────────────

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

// ──────────────────────────────────────────────
// JSON helpers
// ──────────────────────────────────────────────

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func proxyRaw(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(body)
}

// ──────────────────────────────────────────────
// Login & session handlers
// ──────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Username string `json:"username,omitempty"`
	IsAdmin  bool   `json:"is_admin,omitempty"`
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		jsonError(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	isAdmin, err := validateCredentials(req.Username, req.Password)
	if err != nil {
		log.Printf("Login failed for user %q: %v", req.Username, err)
		jsonError(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	tokenStr, err := createToken(req.Username, isAdmin)
	if err != nil {
		log.Printf("Failed to create token: %v", err)
		jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	setCredentialsCookie(w, req.Password)

	jsonResponse(w, loginResponse{
		Message:  "Login successful",
		Username: req.Username,
		IsAdmin:  isAdmin,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "mlflow_creds", Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	jsonResponse(w, map[string]string{"message": "Logged out"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	claims, err := getClaimsFromRequest(r)
	if err != nil {
		jsonError(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	jsonResponse(w, map[string]interface{}{
		"username": claims.Username,
		"is_admin": claims.IsAdmin,
	})
}

func handleMeCredentials(w http.ResponseWriter, r *http.Request) {
	claims, err := getClaimsFromRequest(r)
	if err != nil {
		jsonError(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	password := getPasswordFromToken(r)
	if password == "" {
		jsonError(w, "Session expired", http.StatusUnauthorized)
		return
	}
	jsonResponse(w, map[string]string{
		"username": claims.Username,
		"password": password,
	})
}

// ──────────────────────────────────────────────
// Admin middleware
// ──────────────────────────────────────────────

type adminContext struct {
	claims   *Claims
	password string
}

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

// ──────────────────────────────────────────────
// Reverse proxy for MLflow UI
// ──────────────────────────────────────────────

func newMLflowProxy() *httputil.ReverseProxy {
	target, _ := url.Parse(mlflowURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/mlflow")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		r.Host = "localhost:5000"
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if location := resp.Header.Get("Location"); location != "" {
			if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "/mlflow") {
				resp.Header.Set("Location", "/mlflow"+location)
			}
		}

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "text/html") {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			resp.Body.Close()

			html := string(body)
			html = strings.ReplaceAll(html, `href="/static-files/`, `href="/mlflow/static-files/`)
			html = strings.ReplaceAll(html, `src="/static-files/`, `src="/mlflow/static-files/`)
			html = strings.ReplaceAll(html, `"/ajax-api/`, `"/mlflow/ajax-api/`)

			// Inject portal buttons into MLflow's left sidebar (above Assistant/Docs/Settings)
			portalBar := `<style>
#portal-sidebar-inject{position:fixed;left:0;bottom:128px;width:60px;z-index:99999;display:flex;flex-direction:column;align-items:center;padding-bottom:0;font-family:'Inter',-apple-system,BlinkMacSystemFont,sans-serif;}
#portal-sidebar-inject .p-btn{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:2px;width:100%;padding:8px 4px;cursor:pointer;border:none;background:transparent;text-decoration:none;transition:background .15s;border-radius:0;}
#portal-sidebar-inject .p-btn:hover{background:rgba(255,255,255,0.08);}
#portal-sidebar-inject .p-btn svg{opacity:0.7;}
#portal-sidebar-inject .p-btn:hover svg{opacity:1;}
#portal-sidebar-inject .p-label{font-size:10px;font-weight:500;line-height:1.2;white-space:nowrap;}
#portal-creds-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,0.5);backdrop-filter:blur(4px);z-index:100000;align-items:center;justify-content:center;}
#portal-creds-overlay.active{display:flex;}
#portal-creds-card{background:#1a1a2e;border:1px solid rgba(255,255,255,0.1);border-radius:12px;padding:28px 32px;min-width:340px;box-shadow:0 20px 60px rgba(0,0,0,0.5);font-family:'Inter',-apple-system,sans-serif;animation:pcSlideUp .25s ease;}
@keyframes pcSlideUp{from{opacity:0;transform:translateY(12px);}to{opacity:1;transform:translateY(0);}}
#portal-creds-card h3{margin:0 0 20px;font-size:16px;font-weight:700;color:#fff;display:flex;align-items:center;gap:8px;}
#portal-creds-card .pc-row{margin-bottom:14px;}
#portal-creds-card .pc-lbl{font-size:11px;font-weight:600;color:#8892a4;text-transform:uppercase;letter-spacing:0.5px;margin-bottom:4px;}
#portal-creds-card .pc-val{display:flex;align-items:center;gap:8px;background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.08);border-radius:6px;padding:10px 12px;font-size:14px;color:#e2e8f0;font-family:'SF Mono',Consolas,monospace;word-break:break-all;}
#portal-creds-card .pc-val .pc-copy{margin-left:auto;cursor:pointer;background:none;border:none;color:#64748b;padding:2px;border-radius:4px;display:flex;transition:color .15s;flex-shrink:0;}
#portal-creds-card .pc-val .pc-copy:hover{color:#00e5d0;}
#portal-creds-card .pc-close{width:100%;margin-top:8px;padding:10px;background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.08);border-radius:8px;color:#94a3b8;font-size:13px;font-weight:600;cursor:pointer;transition:all .15s;}
#portal-creds-card .pc-close:hover{background:rgba(255,255,255,0.1);color:#fff;}
</style>
<div id="portal-sidebar-inject">
<a id="portal-admin-btn" href="/" class="p-btn" style="display:none;" title="Admin Panel">
<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#5bb8ff" stroke-width="2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
<span class="p-label" style="color:#5bb8ff;">Admin</span>
</a>
<button class="p-btn" onclick="document.getElementById('portal-creds-overlay').classList.add('active');portalLoadCreds();" title="Show Credentials">
<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fbbf24" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
<span class="p-label" style="color:#fbbf24;">Creds</span>
</button>
<button class="p-btn" onclick="fetch('/api/logout',{method:'POST'}).then(()=>window.location.href='/')" title="Logout">
<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
<span class="p-label" style="color:#f87171;">Logout</span>
</button>
</div>
<div id="portal-creds-overlay" onclick="if(event.target===this)this.classList.remove('active');">
<div id="portal-creds-card">
<h3><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#00e5d0" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg> Your Credentials</h3>
<div class="pc-row"><div class="pc-lbl">Username</div><div class="pc-val"><span id="pc-user">—</span><button class="pc-copy" onclick="navigator.clipboard.writeText(document.getElementById('pc-user').textContent)" title="Copy"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button></div></div>
<div class="pc-row"><div class="pc-lbl">Password</div><div class="pc-val"><span id="pc-pass">—</span><button class="pc-copy" onclick="navigator.clipboard.writeText(document.getElementById('pc-pass').textContent)" title="Copy"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button></div></div>
<button class="pc-close" onclick="document.getElementById('portal-creds-overlay').classList.remove('active');">Close</button>
</div>
</div>
<script>
function portalLoadCreds(){fetch('/api/me/credentials').then(r=>r.json()).then(d=>{document.getElementById('pc-user').textContent=d.username||'—';document.getElementById('pc-pass').textContent=d.password||'—';}).catch(()=>{document.getElementById('pc-user').textContent='Error';document.getElementById('pc-pass').textContent='Error';});}
fetch('/api/me').then(r=>r.json()).then(d=>{if(d.is_admin){document.getElementById('portal-admin-btn').style.display='flex';}}).catch(()=>{});
</script>`
			html = strings.Replace(html, "</body>", portalBar+"</body>", 1)

			resp.Body = io.NopCloser(bytes.NewReader([]byte(html)))
			resp.ContentLength = int64(len(html))
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(html)))
		}

		return nil
	}

	return proxy
}

func authProxyMiddleware(proxy *httputil.ReverseProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := getClaimsFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		password := getPasswordFromToken(r)
		if password == "" {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
			[]byte(claims.Username+":"+password),
		))

		proxy.ServeHTTP(w, r)
	})
}

// ──────────────────────────────────────────────
// Main
// ──────────────────────────────────────────────

func main() {
	loadConfig()

	proxy := newMLflowProxy()

	mux := http.NewServeMux()

	// ── Auth APIs ──
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/logout", handleLogout)
	mux.HandleFunc("/api/me", handleMe)
	mux.HandleFunc("/api/me/credentials", handleMeCredentials)

	// ── OIDC ──
	if oidcHandler, err := NewOIDCHandler(
		cfg.OIDCIssuerURL, cfg.OIDCClientID, cfg.OIDCClientSecret,
		cfg.OIDCRedirectURL, cfg.AuthDBURI,
		cfg.MLflowAdminUser, cfg.MLflowAdminPass, cfg.OIDCAdminUser,
	); err == nil {
		mux.HandleFunc("/api/oidc/login", oidcHandler.HandleLoginRedirect)
		mux.HandleFunc("/api/oidc/callback", oidcHandler.HandleLoginCallback)
		mux.HandleFunc("/api/oidc/exchange", oidcHandler.HandleExchange)
		mux.HandleFunc("/api/oidc/enabled", oidcHandler.HandleEnabled)
		log.Println("✓ OIDC Single Sign-On Enabled")
	} else {
		mux.HandleFunc("/api/oidc/enabled", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
		})
		log.Printf("⚠ OIDC Single Sign-On Disabled: %v", err)
	}

	// ── Admin: User Management ──
	mux.HandleFunc("/api/admin/users/get", handleAdminGetUser)
	mux.HandleFunc("/api/admin/users/create", handleAdminCreateUser)
	mux.HandleFunc("/api/admin/users/delete", handleAdminDeleteUser)
	mux.HandleFunc("/api/admin/users/update-password", handleAdminUpdatePassword)
	mux.HandleFunc("/api/admin/users/update-admin", handleAdminUpdateAdmin)

	// ── Admin: Experiment Permissions ──
	mux.HandleFunc("/api/admin/experiment-permissions/create", handleAdminCreateExpPerm)
	mux.HandleFunc("/api/admin/experiment-permissions/update", handleAdminUpdateExpPerm)
	mux.HandleFunc("/api/admin/experiment-permissions/delete", handleAdminDeleteExpPerm)

	// ── Admin: Model Permissions ──
	mux.HandleFunc("/api/admin/model-permissions/create", handleAdminCreateModelPerm)
	mux.HandleFunc("/api/admin/model-permissions/update", handleAdminUpdateModelPerm)
	mux.HandleFunc("/api/admin/model-permissions/delete", handleAdminDeleteModelPerm)

	// ── Admin: Resource listing ──
	mux.HandleFunc("/api/admin/users", handleAdminListUsers)
	mux.HandleFunc("/api/admin/experiments", handleAdminListExperiments)
	mux.HandleFunc("/api/admin/models", handleAdminListModels)

	// ── MLflow proxy (protected) ──
	mux.Handle("/mlflow/", authProxyMiddleware(proxy))

	// ── Static frontend ──
	frontendDir := cfg.FrontendDir
	fileServer := http.FileServer(http.Dir(frontendDir))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Authenticated user on root → redirect to MLflow (if not admin)
		if r.URL.Path == "/" {
			if claims, err := getClaimsFromRequest(r); err == nil {
				if !claims.IsAdmin {
					http.Redirect(w, r, "/mlflow/", http.StatusTemporaryRedirect)
					return
				}
			}
		}

		// Serve static files
		path := frontendDir + r.URL.Path
		if _, err := os.Stat(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback → serve index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	log.Printf("🚀 MLflow Portal listening on %s", listenAddr)
	log.Printf("   MLflow backend: %s", mlflowURL)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}
