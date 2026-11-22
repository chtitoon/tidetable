package tidal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	authURL  = "https://login.tidal.com/authorize"
	tokenURL = "https://auth.tidal.com/v1/oauth2/token"
)

// Authenticator handles OAuth 2.0 PKCE authentication flow for Tidal
type Authenticator struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURI  string
	accessToken  string
	refreshToken string

	// mu guards the tokens, and serialises refreshes so concurrent callers
	// never spend the same (possibly single-use) refresh token twice.
	mu sync.Mutex
}

// NewAuthenticator creates a new authenticator for Tidal OAuth
func NewAuthenticator(clientID, clientSecret, redirectURI string) *Authenticator {
	return &Authenticator{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
	}
}

// GenerateAuthURL generates the OAuth authorization URL with PKCE challenge
// Returns the full auth URL and the code verifier (needed later for token exchange)
func (a *Authenticator) GenerateAuthURL() (authURL, codeVerifier string, err error) {
	// Generate PKCE code verifier (random string)
	codeVerifier, err = generateCodeVerifier()
	if err != nil {
		return "", "", fmt.Errorf("generating code verifier: %w", err)
	}

	// Generate code challenge from verifier (SHA256 hash)
	codeChallenge := generateCodeChallenge(codeVerifier)

	// Build authorization URL with OAuth parameters
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", a.clientID)
	params.Set("redirect_uri", a.redirectURI)
	params.Set("scope", "collection.read playlists.write")
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	fullAuthURL := fmt.Sprintf("%s?%s", authURL, params.Encode())
	return fullAuthURL, codeVerifier, nil
}

// ExchangeCodeForToken exchanges the authorization code for access and refresh tokens
// This completes the OAuth flow after user authorization
func (a *Authenticator) ExchangeCodeForToken(authCode, codeVerifier string) error {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", authCode)
	data.Set("client_id", a.clientID)
	data.Set("client_secret", a.clientSecret)
	data.Set("redirect_uri", a.redirectURI)
	data.Set("code_verifier", codeVerifier)
	data.Set("scope", "collection.read playlists.write")

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	a.accessToken = tokenResp.AccessToken
	a.refreshToken = tokenResp.RefreshToken

	return nil
}

// RefreshAccessToken uses the refresh token to obtain a new access token
// Called automatically when API returns 401 Unauthorized
func (a *Authenticator) RefreshAccessToken() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshLocked()
}

// RefreshIfStale refreshes the access token unless it has already changed from
// stale, the token a failed request used: when several concurrent requests get
// a 401 at once, only the first one refreshes.
func (a *Authenticator) RefreshIfStale(stale string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.accessToken != stale {
		return nil
	}
	return a.refreshLocked()
}

func (a *Authenticator) refreshLocked() error {
	data := url.Values{}
	data.Set("client_id", a.clientID)
	data.Set("client_secret", a.clientSecret)
	data.Set("refresh_token", a.refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decoding refresh response: %w", err)
	}

	a.accessToken = tokenResp.AccessToken
	// Some refresh responses include a new refresh token
	if tokenResp.RefreshToken != "" {
		a.refreshToken = tokenResp.RefreshToken
	}

	return nil
}

// SetTokens manually sets the access and refresh tokens
// Used when loading existing tokens from storage
func (a *Authenticator) SetTokens(accessToken, refreshToken string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accessToken = accessToken
	a.refreshToken = refreshToken
}

// GetTokens returns the current access and refresh tokens
func (a *Authenticator) GetTokens() (accessToken, refreshToken string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accessToken, a.refreshToken
}

// GetAccessToken returns just the access token (for API requests)
func (a *Authenticator) GetAccessToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accessToken
}

// ===== PKCE Helper Functions =====

// generateCodeVerifier creates a random code verifier for PKCE
// Returns a base64url-encoded random string
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge creates a SHA256 hash of the code verifier
// Returns the base64url-encoded hash
func generateCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
