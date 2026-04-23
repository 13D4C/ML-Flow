package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// ──────────────────────────────────────────────
// JWT
// ──────────────────────────────────────────────

// Claims represents the JWT payload for authenticated sessions.
type Claims struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// createToken generates a signed JWT for the given user.
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

// parseToken validates and parses a JWT string into Claims.
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
// Cookie / credential helpers
// ──────────────────────────────────────────────

// getClaimsFromRequest extracts and validates the JWT from the session cookie.
func getClaimsFromRequest(r *http.Request) (*Claims, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, err
	}
	return parseToken(cookie.Value)
}

// getPasswordFromToken retrieves the stored MLflow password from the credential cookie.
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

// setCredentialsCookie stores the MLflow password in an HTTP-only cookie.
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
// Auth handlers
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

// handleLogin authenticates a user via username/password against MLflow.
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

// handleLogout clears all session cookies.
func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "mlflow_creds", Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	jsonResponse(w, map[string]string{"message": "Logged out"})
}

// handleMe returns the current user's identity from the JWT.
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

// handleMeCredentials returns the current user's MLflow credentials.
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
