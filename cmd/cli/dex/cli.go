package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
)

var (
	// Replace with your Dex server's settings
	clientID     = "your-dex-client-id"
	clientSecret = "your-dex-client-secret"
	dexEndpoint  = oauth2.Endpoint{
		AuthURL:  "http://localhost:5556/auth",
		TokenURL: "https://your-dex-server/token",
	}
	redirectURL = "http://localhost:8080/callback"
	scopes      = []string{"openid", "profile", "email"} // Include other scopes as needed
)

func main() {
	ctx := context.Background()

	// Configure OAuth2 client
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     dexEndpoint,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}

	// Generate authentication URL
	authURL := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("Visit the following URL to authenticate:\n%s\n", authURL)

	// Start a local HTTP server to handle the callback
	code := getAuthCodeFromCallback()

	// Exchange the authorization code for a token
	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Failed to exchange token: %v", err)
	}

	// Display the token and save it to a file
	displayToken(token)
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

	fmt.Println("Waiting for authentication...")
	for code == "" {
	}

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
