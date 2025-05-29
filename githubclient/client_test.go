// SPDX-License-Identifier: MIT

package githubclient_test

import (
	"bytes"
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/esacteksab/gh-actlock/githubclient" // Import the package under test
)

// Helper function to capture log output
func captureLogOutput(fn func()) string {
	var buf bytes.Buffer
	// Save current logger setup
	originalFlags := log.Flags()
	originalOutput := log.Writer()
	log.SetFlags(0) // Remove timestamp/prefix for easier matching
	log.SetOutput(&buf)
	// Restore logger setup after function returns
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	}()
	fn() // Execute the function that logs
	return buf.String()
}

func TestNewClient_WithToken(t *testing.T) {
	// t.Setenv("GITHUB_TOKEN", "fake-test-token")
	os.Getenv("GITHUB_TOKEN")
	// Note: Ideally, mock or control cache path using t.TempDir()
	// For simplicity here, we focus only on the auth part.

	ctx := context.Background()
	var stdoutBuf bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = originalStdout }() // Restore stdout

	client, err := githubclient.NewClient(ctx)

	w.Close()             // Close writer to signal end of writes
	stdoutBuf.ReadFrom(r) // Read captured stdout
	output := stdoutBuf.String()

	require.NoError(t, err)
	require.NotNil(t, client)

	// Check stdout message
	assert.Contains(t, output, "🔧  Authenticated GitHub API access in effect.")

	// Check transport type (simplified check)
	// This requires knowledge of internal structure, might be brittle
	httpClient := client.Client() // Assuming Client() method exists or accessing http.Client directly
	require.NotNil(t, httpClient)
	cachingTransport, ok := httpClient.Transport.(*githubclient.CachingTransport)
	require.True(t, ok, "Transport should be CachingTransport")
	_, ok = cachingTransport.Transport.(*oauth2.Transport) // Check underlying transport
	assert.True(t, ok, "CachingTransport should wrap oauth2.Transport when token is set")
}

func TestNewClient_WithoutToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "") // Ensure token is unset

	ctx := context.Background()
	var stdoutBuf bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = originalStdout }()

	client, err := githubclient.NewClient(ctx)

	w.Close()
	stdoutBuf.ReadFrom(r)
	output := stdoutBuf.String()

	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Contains(
		t,
		output,
		"⚠️  Unauthenticated GitHub API access in effect (lower rate limit).",
	)

	// Check transport type
	httpClient := client.Client()
	require.NotNil(t, httpClient)
	cachingTransport, ok := httpClient.Transport.(*githubclient.CachingTransport)
	require.True(t, ok, "Transport should be CachingTransport")
	_, ok = cachingTransport.Transport.(*oauth2.Transport)
	assert.False(t, ok, "CachingTransport should NOT wrap oauth2.Transport when token is not set")
	// You could add a check for httpcache.Transport if needed
}

func TestPrintRate(t *testing.T) {
	tests := []struct {
		name          string
		rate          *github.Rate
		expectedLogs  []string
		unexpectedLog string // Optional: check that a specific log is NOT present
	}{
		{
			name: "Authenticated",
			rate: &github.Rate{
				Limit:     5000,
				Remaining: 4000,
				Reset:     github.Timestamp{Time: time.Now().Add(10 * time.Minute)},
			},
			expectedLogs: []string{
				"Rate Limit:",
				"4000/5000 remaining",
				"Resets @",
				"Using authenticated rate limits.",
			},
		},
		{
			name: "Unauthenticated",
			rate: &github.Rate{
				Limit:     60,
				Remaining: 50,
				Reset:     github.Timestamp{Time: time.Now().Add(5 * time.Minute)},
			},
			expectedLogs: []string{
				"Rate Limit:",
				"50/60 remaining",
				"Resets @",
				"Using unauthenticated rate limits.",
			},
		},
		{
			name:          "Nil rate",
			rate:          nil,
			expectedLogs:  []string{"Rate limit info unavailable."},
			unexpectedLog: "Rate Limit:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Need internal access to call printRate - if it were public, you could call it directly.
			// If it remains private, you'd test it via PrintRateLimit.
			// Let's assume we make printRate public for testing or test via PrintRateLimit.

			// Example testing via PrintRateLimit:
			var resp *github.Response
			if tt.rate != nil {
				resp = &github.Response{Rate: *tt.rate} // Create response containing the rate
			} // If tt.rate is nil, resp stays nil

			logOutput := captureLogOutput(func() {
				githubclient.PrintRateLimit(resp) // Call the public function
			})

			for _, expected := range tt.expectedLogs {
				assert.Contains(t, logOutput, expected)
			}
			if tt.unexpectedLog != "" {
				assert.NotContains(t, logOutput, tt.unexpectedLog)
			}
		})
	}
}

// Note: Accessing client.Client() might require exporting it or using internal details.
// Testing transport types depends on the structure and visibility.
// This example assumes CachingTransport is exported.
