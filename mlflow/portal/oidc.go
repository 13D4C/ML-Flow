package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCHandler struct {
	IssuerURL       string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	AuthDBURI       string
	MLflowAdminUser string
	MLflowAdminPass string

	Provider   *oidc.Provider
	OAuth2Conf *oauth2.Config
}

func NewOIDCHandler(issuerURL, clientID, clientSecret, redirectURL, authDBURI, adminUser, adminPass string) (*OIDCHandler, error) {
	if issuerURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("missing OIDC credentials")
	}

	provider, err := oidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to init OIDC provider: %w", err)
	}

	oauth2Conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	h := &OIDCHandler{
		IssuerURL:       issuerURL,
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		RedirectURL:     redirectURL,
		AuthDBURI:       authDBURI,
		MLflowAdminUser: adminUser,
		MLflowAdminPass: adminPass,
		Provider:        provider,
		OAuth2Conf:      oauth2Conf,
	}

	if err := h.ensureOIDCTable(); err != nil {
		log.Printf("Warning: failed to ensure OIDC table: %v", err)
	}

	return h, nil
}

func (h *OIDCHandler) ensureOIDCTable() error {
	db, err := sql.Open("postgres", h.AuthDBURI)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS portal_oidc_users (
		oidc_sub TEXT PRIMARY KEY,
		mlflow_username TEXT NOT NULL,
		mlflow_password TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	)`)
	return err
}

func (h *OIDCHandler) generatePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

func (h *OIDCHandler) getOrCreateOIDCUser(oidcSub, preferredUsername string) (string, string, error) {
	db, err := sql.Open("postgres", h.AuthDBURI)
	if err != nil {
		return "", "", fmt.Errorf("db connect: %w", err)
	}
	defer db.Close()

	var storedUser, storedPass string
	err = db.QueryRow("SELECT mlflow_username, mlflow_password FROM portal_oidc_users WHERE oidc_sub = $1", oidcSub).Scan(&storedUser, &storedPass)
	if err == nil {
		return storedUser, storedPass, nil
	}
	if err != sql.ErrNoRows {
		return "", "", fmt.Errorf("query oidc user: %w", err)
	}

	password := h.generatePassword(20)
	mlflowUsername := preferredUsername

	// Try create user in MLflow
	status, body, err := mlflowRequest("POST", "/api/2.0/mlflow/users/create",
		map[string]string{"username": mlflowUsername, "password": password},
		h.MLflowAdminUser, h.MLflowAdminPass)
	if err != nil {
		return "", "", fmt.Errorf("create mlflow user: %w", err)
	}

	// Update password if user already exists
	if status == 400 && string(body) != "" { 
		// Simulating check for exists since we may not match exact error byte array without bytes package easily here
		_, _, _ = mlflowRequest("PATCH", "/api/2.0/mlflow/users/update-password",
			map[string]string{"username": mlflowUsername, "password": password},
			h.MLflowAdminUser, h.MLflowAdminPass)
	}

	_, err = db.Exec(
		"INSERT INTO portal_oidc_users (oidc_sub, mlflow_username, mlflow_password) VALUES ($1, $2, $3) ON CONFLICT (oidc_sub) DO UPDATE SET mlflow_password = $3",
		oidcSub, mlflowUsername, password)
	if err != nil {
		return "", "", fmt.Errorf("store oidc mapping: %w", err)
	}

	return mlflowUsername, password, nil
}

func (h *OIDCHandler) HandleLoginRedirect(w http.ResponseWriter, r *http.Request) {
	state := h.generatePassword(16)
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	http.Redirect(w, r, h.OAuth2Conf.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

func (h *OIDCHandler) HandleLoginCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oidc_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		jsonError(w, "Invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/", MaxAge: -1})

	// Check if OIDC provider returned an error instead of a code
	if oidcErr := r.URL.Query().Get("error"); oidcErr != "" {
		desc := r.URL.Query().Get("error_description")
		jsonError(w, fmt.Sprintf("OIDC Provider error: %s - %s", oidcErr, desc), http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		jsonError(w, "No code provided in callback URL", http.StatusBadRequest)
		return
	}

	token, err := h.OAuth2Conf.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("OIDC exchange failed: %v", err)
		jsonError(w, "Failed to exchange code", http.StatusUnauthorized)
		return
	}

	userInfo, err := h.Provider.UserInfo(context.Background(), oauth2.StaticTokenSource(token))
	if err != nil {
		log.Printf("OIDC userinfo failed: %v", err)
		jsonError(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	var claims struct {
		Sub               string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
	}
	if err := userInfo.Claims(&claims); err != nil {
		jsonError(w, "Failed to parse claims", http.StatusInternalServerError)
		return
	}

	mlflowUsername := claims.PreferredUsername
	if mlflowUsername == "" {
		mlflowUsername = claims.Email
	}
	if mlflowUsername == "" {
		mlflowUsername = claims.Sub
	}

	mlflowUsername, password, err := h.getOrCreateOIDCUser(claims.Sub, mlflowUsername)
	if err != nil {
		log.Printf("OIDC provision failed: %v", err)
		jsonError(w, "Failed to provision user", http.StatusInternalServerError)
		return
	}

	isAdmin, err := validateCredentials(mlflowUsername, password)
	if err != nil {
		log.Printf("OIDC credential validation failed: %v", err)
		jsonError(w, "Credential validation failed", http.StatusInternalServerError)
		return
	}

	tokenStr, err := createToken(mlflowUsername, isAdmin)
	if err != nil {
		jsonError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tokenStr, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400,
	})
	setCredentialsCookie(w, password)

	if isAdmin {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	} else {
		http.Redirect(w, r, "/mlflow/", http.StatusTemporaryRedirect)
	}
}

func (h *OIDCHandler) HandleEnabled(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": true})
}

// HandleExchange is called by the frontend after the OIDC provider redirects to
// the frontend callback page (e.g. /entry/callback). The frontend forwards the
// code and state query params here so the backend can complete the token exchange.
func (h *OIDCHandler) HandleExchange(w http.ResponseWriter, r *http.Request) {
	// Validate state against cookie
	stateCookie, err := r.Cookie("oidc_state")
	stateParam := r.URL.Query().Get("state")
	if err != nil || stateCookie.Value != stateParam {
		jsonError(w, "Invalid state", http.StatusBadRequest)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/", MaxAge: -1})

	// Check if OIDC provider returned an error
	if oidcErr := r.URL.Query().Get("error"); oidcErr != "" {
		desc := r.URL.Query().Get("error_description")
		jsonError(w, fmt.Sprintf("OIDC Provider error: %s - %s", oidcErr, desc), http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		jsonError(w, "No code provided", http.StatusBadRequest)
		return
	}

	token, err := h.OAuth2Conf.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("OIDC exchange failed: %v", err)
		jsonError(w, "Failed to exchange code", http.StatusUnauthorized)
		return
	}

	userInfo, err := h.Provider.UserInfo(context.Background(), oauth2.StaticTokenSource(token))
	if err != nil {
		log.Printf("OIDC userinfo failed: %v", err)
		jsonError(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	var claims struct {
		Sub               string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
	}
	if err := userInfo.Claims(&claims); err != nil {
		jsonError(w, "Failed to parse claims", http.StatusInternalServerError)
		return
	}

	mlflowUsername := claims.PreferredUsername
	if mlflowUsername == "" {
		mlflowUsername = claims.Email
	}
	if mlflowUsername == "" {
		mlflowUsername = claims.Sub
	}

	mlflowUsername, password, err := h.getOrCreateOIDCUser(claims.Sub, mlflowUsername)
	if err != nil {
		log.Printf("OIDC provision failed: %v", err)
		jsonError(w, "Failed to provision user", http.StatusInternalServerError)
		return
	}

	isAdmin, err := validateCredentials(mlflowUsername, password)
	if err != nil {
		log.Printf("OIDC credential validation failed: %v", err)
		jsonError(w, "Credential validation failed", http.StatusInternalServerError)
		return
	}

	tokenStr, err := createToken(mlflowUsername, isAdmin)
	if err != nil {
		jsonError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tokenStr, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400,
	})
	setCredentialsCookie(w, password)

	// Return JSON so the frontend can decide where to redirect
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":       true,
		"is_admin": isAdmin,
	})
}

