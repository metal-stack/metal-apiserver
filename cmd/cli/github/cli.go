package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// Configuration for the OAuth2 client
var (
	clientID       = "your-github-client-id"
	clientSecret   = "your-github-client-secret"
	oauth2Endpoint = github.Endpoint               // Use GitHub's predefined endpoint
	scopes         = []string{"read:user", "repo"} // Adjust scopes as needed
)

func main() {
	ctx := context.Background()

	// Configure OAuth2 config
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2Endpoint,
		Scopes:       scopes,
		RedirectURL:  "http://localhost:8080/callback",
	}

	// Generate the authentication URL
	url := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("Visit the following URL to authenticate: \n%s\n", url)

	// Start a local HTTP server to handle the callback
	code := getAuthCodeFromCallback()

	// Exchange the authorization code for a token
	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Failed to exchange token: %v", err)
	}

	// Display the JWT token
	displayToken(token)

	// Save the token to a file
	saveTokenToFile(token, "token.json")
}

// getAuthCodeFromCallback starts a temporary HTTP server to capture the auth code
func getAuthCodeFromCallback() string {
	var code string
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code = r.URL.Query().Get("code")
		fmt.Fprintf(w, "Authentication successful! You can close this window.")
	})

	server := &http.Server{Addr: ":8080"}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for the user to authenticate and the callback to fire
	fmt.Println("Waiting for authentication...")
	for code == "" {
	}

	// Shutdown the server after capturing the code
	server.Shutdown(context.Background())
	return code
}

// displayToken prints the token to the console
func displayToken(token *oauth2.Token) {
	fmt.Println("Access Token:", token.AccessToken)
	fmt.Println("Refresh Token:", token.RefreshToken)
	fmt.Println("Token Type:", token.TokenType)
	fmt.Println("Expiry:", token.Expiry)
}

// saveTokenToFile saves the token to a file in JSON format
func saveTokenToFile(token *oauth2.Token, filename string) {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create token file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(token); err != nil {
		log.Fatalf("Failed to save token to file: %v", err)
	}

	fmt.Printf("Token saved to %s\n", filename)
}
