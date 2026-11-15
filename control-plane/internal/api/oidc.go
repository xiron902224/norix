package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
	"golang.org/x/oauth2"
)

func publicBaseURL(request *http.Request) string {
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("NORIX_PUBLIC_URL")), "/"); base != "" {
		return base
	}
	scheme := "http"
	if request.TLS != nil || strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := request.Host
	if host == "" {
		host = "127.0.0.1:8080"
	}
	return scheme + "://" + host
}

func webOrigin() string {
	if origin := strings.TrimRight(strings.TrimSpace(os.Getenv("NORIX_WEB_ORIGIN")), "/"); origin != "" {
		return origin
	}
	if origin := strings.TrimRight(strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")), "/"); origin != "" {
		parts := strings.Split(origin, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	return "http://127.0.0.1:5173"
}

func oidcRedirectURI(request *http.Request) string {
	return publicBaseURL(request) + "/auth/oidc/callback"
}

func randomURLToken(nbytes int) (string, error) {
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// OIDCProvidersHandler lists enabled OIDC providers for the login UI (no secrets).
func OIDCProvidersHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		items, err := db.ListEnabledOIDCProviders(request.Context(), pool)
		if err != nil {
			http.Error(response, "could not list providers", http.StatusInternalServerError)
			return
		}
		type publicProvider struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		out := make([]publicProvider, 0, len(items))
		for _, item := range items {
			out = append(out, publicProvider{ID: item.ID, Name: item.Name})
		}
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(response).Encode(out)
	}
}

// OIDCStartHandler redirects the browser to the IdP authorization endpoint.
func OIDCStartHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		providerID := strings.TrimSpace(request.URL.Query().Get("provider_id"))
		if providerID == "" {
			http.Error(response, "provider_id is required", http.StatusBadRequest)
			return
		}
		provider, err := db.GetSSOProviderByID(request.Context(), pool, providerID)
		if err != nil || !provider.Enabled || provider.Kind != "oidc" {
			http.Error(response, "oidc provider not found or disabled", http.StatusNotFound)
			return
		}
		if provider.IssuerURL == "" || provider.ClientID == "" {
			http.Error(response, "oidc provider is misconfigured", http.StatusFailedDependency)
			return
		}
		secret := strings.TrimSpace(provider.ClientSecretEnc)
		if secret == "" || secret == " " {
			http.Error(response, "oidc client secret is not configured", http.StatusFailedDependency)
			return
		}

		oauth2Config, verifier, err := buildOIDCConfig(request.Context(), provider, oidcRedirectURI(request))
		if err != nil {
			http.Error(response, "oidc discovery failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		_ = verifier

		state, err := randomURLToken(32)
		if err != nil {
			http.Error(response, "could not create state", http.StatusInternalServerError)
			return
		}
		nonce, err := randomURLToken(32)
		if err != nil {
			http.Error(response, "could not create nonce", http.StatusInternalServerError)
			return
		}
		if err := db.StoreOIDCLoginState(request.Context(), pool, db.OIDCLoginState{
			State:          state,
			Nonce:          nonce,
			ProviderID:     provider.ID,
			OrganizationID: provider.OrganizationID,
			ExpiresAt:      time.Now().UTC().Add(10 * time.Minute),
		}); err != nil {
			http.Error(response, "could not persist oidc state", http.StatusInternalServerError)
			return
		}

		authURL := oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.AccessTypeOffline)
		http.Redirect(response, request, authURL, http.StatusFound)
	}
}

// OIDCCallbackHandler exchanges the authorization code, creates a session, and redirects to the web UI.
func OIDCCallbackHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if errMsg := request.URL.Query().Get("error"); errMsg != "" {
			desc := request.URL.Query().Get("error_description")
			http.Error(response, fmt.Sprintf("oidc error: %s (%s)", errMsg, desc), http.StatusBadRequest)
			return
		}
		code := strings.TrimSpace(request.URL.Query().Get("code"))
		stateToken := strings.TrimSpace(request.URL.Query().Get("state"))
		if code == "" || stateToken == "" {
			http.Error(response, "code and state are required", http.StatusBadRequest)
			return
		}
		loginState, err := db.ConsumeOIDCLoginState(request.Context(), pool, stateToken)
		if err != nil {
			http.Error(response, "invalid or expired oidc state", http.StatusBadRequest)
			return
		}
		provider, err := db.GetSSOProviderByID(request.Context(), pool, loginState.ProviderID)
		if err != nil || !provider.Enabled || provider.Kind != "oidc" {
			http.Error(response, "oidc provider not found or disabled", http.StatusNotFound)
			return
		}

		oauth2Config, verifier, err := buildOIDCConfig(request.Context(), provider, oidcRedirectURI(request))
		if err != nil {
			http.Error(response, "oidc discovery failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		token, err := oauth2Config.Exchange(request.Context(), code)
		if err != nil {
			http.Error(response, "oidc token exchange failed", http.StatusBadGateway)
			return
		}
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok || rawIDToken == "" {
			http.Error(response, "oidc id_token missing", http.StatusBadGateway)
			return
		}
		idToken, err := verifier.Verify(request.Context(), rawIDToken)
		if err != nil {
			http.Error(response, "oidc id_token verification failed", http.StatusUnauthorized)
			return
		}
		if idToken.Nonce != loginState.Nonce {
			http.Error(response, "oidc nonce mismatch", http.StatusUnauthorized)
			return
		}

		var claims struct {
			Email             string `json:"email"`
			EmailVerified     bool   `json:"email_verified"`
			Name              string `json:"name"`
			PreferredUsername string `json:"preferred_username"`
		}
		if err := idToken.Claims(&claims); err != nil {
			http.Error(response, "oidc claims decode failed", http.StatusBadGateway)
			return
		}
		email := strings.TrimSpace(strings.ToLower(claims.Email))
		if email == "" && strings.Contains(claims.PreferredUsername, "@") {
			email = strings.TrimSpace(strings.ToLower(claims.PreferredUsername))
		}
		if email == "" {
			http.Error(response, "oidc token did not include an email claim", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(claims.Name)
		if name == "" {
			name = claims.PreferredUsername
		}

		user, err := db.FindOrCreateSSOUser(request.Context(), pool, provider.OrganizationID, email, name)
		if err != nil {
			http.Error(response, "could not provision user", http.StatusInternalServerError)
			return
		}

		sessionToken, tokenHash, err := auth.NewSessionToken()
		if err != nil {
			http.Error(response, "could not create session", http.StatusInternalServerError)
			return
		}
		expiresAt := time.Now().UTC().Add(12 * time.Hour)
		if err := db.CreateSession(request.Context(), pool, user.OrganizationID, user.ID, tokenHash, expiresAt); err != nil {
			http.Error(response, "could not create session", http.StatusInternalServerError)
			return
		}

		redirect := webOrigin() + "/login?sso_token=" + url.QueryEscape(sessionToken)
		http.Redirect(response, request, redirect, http.StatusFound)
	}
}

func buildOIDCConfig(ctx context.Context, provider db.SSOProvider, redirectURI string) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	issuer := strings.TrimRight(strings.TrimSpace(provider.IssuerURL), "/")
	oidcProvider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, nil, err
	}
	secret := strings.TrimSpace(provider.ClientSecretEnc)
	oauth2Config := &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: secret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	verifier := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID})
	return oauth2Config, verifier, nil
}
