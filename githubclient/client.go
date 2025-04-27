// SPDX-License-Identifier: MIT
package githubclient

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-github/v71/github"
	"golang.org/x/oauth2"

	"github.com/esacteksab/httpcache"
	"github.com/esacteksab/httpcache/diskcache"
)

// SHALength is the standard length of a Git SHA-1 hash.
const SHALength = 40

// isHexDigit checks if a byte is a valid hexadecimal digit (0-9, a-f, A-F).
//
// - b: The byte to check.
// Returns: true if the byte is a valid hex digit, false otherwise.
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// IsHexString checks if a string consists entirely of valid hexadecimal digits.
// This is used to determine if a string is likely a Git SHA.
//
// - s: The string to check.
// Returns: true if the string contains only hexadecimal characters, false otherwise.
func IsHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

// CachingTransport wraps an http.RoundTripper to potentially add custom logic,
// such as logging or metrics, around the transport (including the cache layer).
type CachingTransport struct {
	Transport http.RoundTripper // The underlying transport, which could be the cache transport or an authenticated transport.
}

// RoundTrip executes a single HTTP transaction, passing it to the wrapped Transport.
// This method satisfies the http.RoundTripper interface.
//
// - req: The HTTP request to execute.
// Returns: The HTTP response and an error, if any.
func (t *CachingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Optional logging or request modification can be added here before the request is sent
	// to the wrapped transport (which might be the cache transport).
	// fmt.Printf("Performing HTTP request: %s %s\n", req.Method, req.URL.String()) // Example logging

	// Delegate the actual request execution to the wrapped transport.
	return t.Transport.RoundTrip(req)
}

// NewClient initializes and returns a new GitHub API client.
// It configures authentication (using GITHUB_TOKEN if available) and adds an HTTP cache layer.
//
// - ctx: The context for the client, allows for cancellation.
// Returns: An initialized *github.Client and an error if setup fails (e.g., cache directory creation).
func NewClient(ctx context.Context) (*github.Client, error) {
	// Get the user's cache directory (platform-specific).
	// This is where we'll store cached HTTP responses to reduce API calls.
	projectCacheDir, err := os.UserCacheDir()
	if err != nil {
		// Return an error if the user cache directory cannot be determined.
		return nil, fmt.Errorf("failed to get user cache directory: %w", err)
	}

	// Define the subdirectory name within the user cache directory for this application.
	appCacheDirName := "gh-actlock"
	// Construct the full path for the application's cache directory.
	cachePath := filepath.Join(projectCacheDir, appCacheDirName)

	// Create the cache directory if it doesn't exist. 0o750 is the permission
	// mode in octal notation: Owner: read/write/execute (7) Group: read/execute
	// (5) Others: no access (0)
	if err := os.MkdirAll(cachePath, 0o750); err != nil { //nolint:mnd
		// Return an error if the cache directory cannot be created.
		return nil, fmt.Errorf("could not create cache directory '%s': %w", cachePath, err)
	}

	// Initialize the disk cache using the specified path.
	// This cache will store HTTP responses to reduce API calls.
	cache := diskcache.New(cachePath)

	// Get the GitHub token from the environment variable.
	// Using an environment variable is more secure than hardcoding the token.
	token := os.Getenv("GITHUB_TOKEN")

	var httpClient *http.Client // Variable to hold the final configured HTTP client.
	// Initialize an HTTP transport that uses the disk cache.
	cacheTransport := httpcache.NewTransport(cache)

	// Check if a GitHub token was found.
	if token != "" {
		fmt.Println("🔧 Using GITHUB_TOKEN for authentication.")
		// Create an OAuth2 token source with the provided token.
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		// Create an OAuth2 transport that wraps the cache transport and adds the token to requests.
		// This allows authenticated requests to be cached.
		authTransport := &oauth2.Transport{
			Base:   cacheTransport,                   // The transport to wrap (our cache transport).
			Source: oauth2.ReuseTokenSource(nil, ts), // Source for the token, reusing it.
		}
		// Wrap the authenticated transport with our custom CachingTransport.
		// This allows us to add custom logic around HTTP requests if needed.
		cachingTransport := &CachingTransport{Transport: authTransport}
		// Create the final HTTP client using the wrapped authenticated transport.
		httpClient = &http.Client{Transport: cachingTransport}
	} else {
		fmt.Println("⚠️  No GITHUB_TOKEN found, using unauthenticated requests (lower rate limit).")
		// If no token is found, use the cache transport directly wrapped in our custom transport.
		// Unauthenticated requests have much lower rate limits (60/hour vs 5000/hour).
		debugTransport := &CachingTransport{Transport: cacheTransport}
		// Create the final HTTP client using the wrapped cache transport.
		httpClient = &http.Client{Transport: debugTransport}
	}

	// Create and return the GitHub client using the configured HTTP client.
	client := github.NewClient(httpClient)
	return client, nil
}

// CheckRateLimit retrieves the current GitHub API rate limit status and logs it.
// This is useful for monitoring usage and diagnosing rate limit errors.
//
// - ctx: The context for the API call, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
func CheckRateLimit(ctx context.Context, client *github.Client) {
	// Call the GitHub API to get the rate limits.
	// GitHub provides separate rate limits for different API endpoints.
	limits, resp, err := client.RateLimit.Get(ctx)
	if err != nil {
		// Log a warning if retrieving rate limits fails.
		log.Printf("Warning: Could not retrieve rate limits: %v", err)
		// Even if the API call failed, the 'resp' might contain rate limit headers.
		// Attempt to print rate limit info from the response headers as a fallback.
		PrintRateLimit(resp)
		return
	}
	// If the call succeeded and limit data is available, print the core limit.
	// The "core" limit applies to most GitHub API endpoints.
	if limits != nil && limits.Core != nil {
		printRate(limits.Core)
	} else {
		// Log a warning if the returned data structure doesn't contain expected rate limit info.
		log.Printf("Warning: Rate limit data not available in response.")
	}
}

// PrintRateLimit logs rate limit information extracted directly from a GitHub API Response.
// This function is primarily used as a fallback if retrieving the full RateLimit struct fails.
//
// - resp: The *github.Response object from a GitHub API call.
func PrintRateLimit(resp *github.Response) {
	// If the response object itself is nil, call printRate with a nil rate object.
	if resp == nil {
		printRate(nil) // printRate will log "Rate limit info unavailable."
		return
	}
	// If the response is not nil, pass the address of its Rate field to printRate.
	// The github.Response.Rate field contains limit details from the response headers.
	printRate(&resp.Rate)
}

// printRate logs the details of a specific rate limit struct.
// It formats the remaining requests, total limit, and reset time.
//
// - rate: A pointer to the github.Rate struct containing limit details.
func printRate(rate *github.Rate) {
	// Check if the rate struct is nil (e.g., if called with a nil response).
	if rate == nil {
		log.Printf("Rate limit info unavailable.")
		return
	}
	// Format the reset time from UTC to the local timezone and a readable string.
	// The rate.Reset field contains the Unix timestamp when the rate limit resets.
	resetTime := rate.Reset.Time.Local().Format("15:04:05 MST")
	// Log the rate limit details: remaining requests, total limit, and reset time.
	log.Printf("Rate Limit: %d/%d remaining | Resets @ %s", rate.Remaining, rate.Limit, resetTime)

	// Provide additional context based on the identified rate limit.
	const authenticatedLimit = 5000 // Typical authenticated rate limit per hour.
	const unauthenticatedLimit = 60 // Typical unauthenticated rate limit per hour.
	if rate.Limit >= authenticatedLimit {
		log.Println("  Using authenticated rate limits.")
	} else if rate.Limit <= unauthenticatedLimit {
		log.Println("  Using unauthenticated rate limits.")
	}
}
