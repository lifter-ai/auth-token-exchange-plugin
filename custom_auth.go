package auth_token_exchange_plugin

import (
    "context"
    "fmt"
    "net/http"
    "encoding/json"
    "strings"
    "time"
    "net/url"
    "math/rand"
)

// Config the plugin configuration.
type Config struct {
    AuthURL string `json:"authURL,omitempty"`
    Production  bool   `json:"production,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
    return &Config{
        Production: false,
    }
}

// CustomAuth a plugin.
type CustomAuth struct {
    next        http.Handler
    authURL string
    name        string
    production  bool
}

// New created a new CustomAuth plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
    if config.AuthURL == "" {
        return nil, fmt.Errorf("AuthURL must be set")
    }

    // Validate URL
    _, err := url.Parse(config.AuthURL)
    if err != nil {
        return nil, fmt.Errorf("invalid AuthURL: %v", err)
    }

    return &CustomAuth{
        next:        next,
        authURL: config.AuthURL,
        name:        name,
        production:  config.Production,
    }, nil
}

func (a *CustomAuth) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
    authHeader := req.Header.Get("Authorization")
    if authHeader == "" {
        http.Error(rw, "Missing Authorization header", http.StatusUnauthorized)
        return
    }

    // Check for test token
    if !a.production && strings.TrimPrefix(authHeader, "Bearer ") == "test-token" {
        // For test token, return 200 OK without forwarding the request
        rw.WriteHeader(http.StatusOK)
        return
    }

    // Real authentication logic
    client := &http.Client{Timeout: 10 * time.Second}
    verifyReq, err := http.NewRequest("GET", a.authURL, nil)
    if err != nil {
        logError(fmt.Sprintf("Failed to create request: %v", err))
        http.Error(rw, "Internal server error", http.StatusInternalServerError)
        return
    }
    verifyReq.Header.Set("Authorization", authHeader)

 var resp *http.Response
    var retries int
    backoff := 100 * time.Millisecond
    for retries < 3 {
        resp, err = client.Do(verifyReq)
        if err == nil {
            break
        }
        retries++
        
        // Calculate jitter
        jitter := time.Duration(rand.Int63n(int64(backoff)))
        sleepTime := backoff + jitter
        
        logError(fmt.Sprintf("Request failed (attempt %d): %v. Retrying in %v", retries, err, sleepTime))
        
        time.Sleep(sleepTime)
        
        // Exponential backoff
        backoff *= 2
    }

    if err != nil {
        logError(fmt.Sprintf("Failed to reach users-api after %d retries: %v", retries, err))
        http.Error(rw, "Failed to reach users-api", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusUnauthorized {
        http.Error(rw, "Invalid token", http.StatusUnauthorized)
        return
    }

    if resp.StatusCode != http.StatusOK {
        logError(fmt.Sprintf("Unexpected response from users-api: %d", resp.StatusCode))
        http.Error(rw, "Unexpected response from users-api", resp.StatusCode)
        return
    }

    var userInfo map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
        logError(fmt.Sprintf("Failed to decode user info: %v", err))
        http.Error(rw, "Failed to process user info", http.StatusInternalServerError)
        return
    }

    userInfoJSON, err := json.Marshal(userInfo)
    if err != nil {
        logError(fmt.Sprintf("Failed to marshal user info: %v", err))
        http.Error(rw, "Failed to process user info", http.StatusInternalServerError)
        return
    }

    encodedUserInfo := base64.StdEncoding.EncodeToString(userInfoJSON)
    req.Header.Set("X-User-Info", encodedUserInfo)

    // Remove original Authorization header
    req.Header.Del("Authorization")

    a.next.ServeHTTP(rw, req)
}

func logError(msg string) {
    fmt.Printf("CustomAuth plugin error: %s\n", msg)
}