// SPDX-License-Identifier: MIT
package githubclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-github/v72/github"
	"golang.org/x/oauth2"

	"github.com/esacteksab/httpcache"
	"github.com/esacteksab/httpcache/diskcache"

	"github.com/esacteksab/gh-actlock/utils"
)

// SHALength is the standard length of a Git SHA-1 hash.
const (
	SHALength   = 40
	authLimit   = 5000
	unAuthLimit = 60
)

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

	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		authTransport := &oauth2.Transport{
			Base:   cacheTransport,
			Source: oauth2.ReuseTokenSource(nil, ts),
		}
		cachingTransport := &CachingTransport{Transport: authTransport}
		httpClient = &http.Client{Transport: cachingTransport}
	} else {
		debugTransport := &CachingTransport{Transport: cacheTransport}
		httpClient = &http.Client{Transport: debugTransport}
	}

	client := github.NewClient(httpClient)

	// After client creation, check and log the actual rate limit/auth status:
	limitType := CheckRateLimit(ctx, client)
	utils.LogRateLimitStatus(limitType)

	return client, nil
}

// CheckRateLimit retrieves the current GitHub API rate limit status and logs it.
// This is useful for monitoring usage and diagnosing rate limit errors.
//
// - ctx: The context for the API call, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
//
// Returns a string representing the state of authentication.
func CheckRateLimit(ctx context.Context, client *github.Client) string {
	limits, resp, err := client.RateLimit.Get(ctx)
	if err != nil {
		PrintRateLimit(resp)
		return "unknown"
	}
	if limits != nil && limits.Core != nil {
		printRate(limits.Core)
		switch {
		case limits.Core.Limit >= authLimit:
			return "authenticated"
		case limits.Core.Limit <= unAuthLimit:
			return "unauthenticated"
		default:
			return "unknown"
		}
	}
	utils.Logger.Debugf("Warning: Rate limit data not available in response.")
	return "unknown"
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
		utils.Logger.Debugf("Rate limit info unavailable.")
		return
	}
	// Format the reset time from UTC to the local timezone and a readable string.
	// The rate.Reset field contains the Unix timestamp when the rate limit resets.
	resetTime := rate.Reset.Time.Local().Format("15:04:05 MST")
	// Log the rate limit details: remaining requests, total limit, and reset time.
	utils.Logger.Debugf(
		"Rate Limit: %d/%d remaining | Resets @ %s",
		rate.Remaining,
		rate.Limit,
		resetTime,
	)

	// Provide additional context based on the identified rate limit.
	const authenticatedLimit = 5000 // Typical authenticated rate limit per hour.
	const unauthenticatedLimit = 60 // Typical unauthenticated rate limit per hour.
	if rate.Limit >= authenticatedLimit {
		utils.Logger.Debugf("  Using authenticated rate limits.")
	} else if rate.Limit <= unauthenticatedLimit {
		utils.Logger.Debugf("  Using unauthenticated rate limits.")
	}
}

// GetLatestActionRef retrieves the most recent reference (tag or release) for a GitHub repository
// and its corresponding commit SHA. It first attempts to get the latest release, and if that fails,
// falls back to the most recent tag.
//
// - ctx: The context for API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - owner: The owner (user or organization) of the GitHub repository.
// - repo: The name of the GitHub repository.
// Returns:
//   - string: The name of the latest reference (tag or release name)
//   - string: The full SHA hash corresponding to that reference
//   - error: An error if both release and tag retrieval fail
func GetLatestActionRef(
	ctx context.Context,
	client *github.Client,
	owner string,
	repo string,
) (string, string, error) {
	// First try to get the latest release as it's usually more stable
	// Releases are formally published versions, often with release notes and assets
	release, _, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err == nil && release != nil && release.TagName != nil {
		// If we found a release, get the commit SHA that the release tag points to
		sha, err := ResolveRefToSHA(ctx, client, owner, repo, *release.TagName)
		if err == nil {
			// Return the release tag name and its corresponding commit SHA
			return *release.TagName, sha, nil
		}
		// If we couldn't get the SHA for the release tag, continue to try regular tags
		// This can happen if the release references a lightweight tag that doesn't exist as a full ref
	}

	// If no releases exist or there was an error getting them, fall back to listing tags
	// Tags are simpler reference points that may or may not be associated with a release

	// Set up pagination options to limit to the 10 most recent tags
	opt := &github.ListOptions{PerPage: 10} //nolint:mnd

	// Retrieve the list of tags for the repository
	tags, _, err := client.Repositories.ListTags(ctx, owner, repo, opt)
	if err != nil {
		return "", "", fmt.Errorf("error getting tags for %s/%s: %w", owner, repo, err)
	}

	// Check if any tags were found
	if len(tags) == 0 {
		return "", "", fmt.Errorf("no tags found for %s/%s", owner, repo)
	}

	// Use the first tag in the list, which is typically the most recent
	// GitHub's API returns tags in reverse chronological order (newest first)
	latestTag := tags[0]

	// Validate that the tag contains all the data we need
	// Tags should have a name and a commit SHA, but we check to be safe
	if latestTag.Name == nil || latestTag.Commit == nil || latestTag.Commit.SHA == nil {
		return "", "", fmt.Errorf("invalid tag data for %s/%s", owner, repo)
	}

	// Return the tag name and its corresponding commit SHA
	return *latestTag.Name, *latestTag.Commit.SHA, nil
}
