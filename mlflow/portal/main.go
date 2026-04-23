package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

// ──────────────────────────────────────────────
// Entry point
// ──────────────────────────────────────────────
//
// File structure:
//   config.go    — Configuration loading from environment variables
//   auth.go      — JWT, cookies, login/logout/me handlers
//   admin.go     — Admin middleware + user/permission management handlers
//   oidc.go      — OIDC Single Sign-On handler
//   mlflow.go    — MLflow API client (validateCredentials, mlflowRequest)
//   proxy.go     — Reverse proxy for MLflow UI + auth proxy middleware
//   response.go  — JSON response helpers (jsonResponse, jsonError, proxyRaw)

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
