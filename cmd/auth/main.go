package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/chtitoon/tidetable/tidal"
)

const redirectURI = "http://localhost:8080/callback"

func main() {
	clientID := os.Getenv("TIDAL_CLIENT_ID")
	clientSecret := os.Getenv("TIDAL_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Fatal("TIDAL_CLIENT_ID and TIDAL_CLIENT_SECRET must be set")
	}

	client := tidal.NewClient(clientID, clientSecret, redirectURI)

	authURL, codeVerifier, err := client.GenerateAuthURL()
	if err != nil {
		log.Fatalf("Failed to generate auth URL: %v", err)
	}

	fmt.Println("\n=== Tidal OAuth Authentication ===")
	fmt.Printf("Opening browser for authentication...\n")
	fmt.Printf("If the browser doesn't open, visit this URL manually:\n%s\n\n", authURL)

	// Open browser
	openBrowser(authURL)

	// Start local server to receive callback
	authCodeChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	server := &http.Server{Addr: ":8080"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errorChan <- fmt.Errorf("no authorization code received")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Error: No authorization code received")
			return
		}

		authCodeChan <- code
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Authentication successful! You can close this window and return to the terminal.")
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errorChan <- err
		}
	}()

	fmt.Println("Waiting for authentication...")

	select {
	case authCode := <-authCodeChan:
		// Shutdown server
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)

		// Exchange code for token
		if err := client.ExchangeCodeForToken(authCode, codeVerifier); err != nil {
			log.Fatalf("Failed to exchange code for token: %v", err)
		}

		accessToken, refreshToken := client.GetTokens()

		fmt.Printf("\n=== Authentication Successful! ===\n")
		fmt.Printf("Add these to your .env file or GitHub Secrets:\n\n")
		fmt.Printf("TIDAL_ACCESS_TOKEN=%s\n", accessToken)
		fmt.Printf("TIDAL_REFRESH_TOKEN=%s\n", refreshToken)

	case err := <-errorChan:
		log.Fatalf("Authentication failed: %v", err)

	case <-time.After(5 * time.Minute):
		log.Fatal("Authentication timed out after 5 minutes")
	}
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		log.Printf("Failed to open browser automatically: %v", err)
	}
}
