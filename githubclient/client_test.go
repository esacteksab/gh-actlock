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
	"github.com/esacteksab/gh-actlock/utils"
)

// Helper function to capture log output from utils.Logger (charmbracelet/log)
func captureLogOutput(fn func()) string {
	var buf bytes.Buffer

	if utils.Logger == nil {
		utils.CreateLogger(true) // Fallback initialization
	}

	// charmbracelet/log.Logger doesn't have direct getters for its current output.
	// We will set the output to our buffer for capture.
	// The original output writer is known to be os.Stderr from utils.CreateLogger.

	// Save the current configuration for restoration if possible, or restore to known defaults.
	// For charmbracelet/log, we'll restore to the typical verbose state.
	// These are the settings typically set by utils.CreateLogger(true)
	restoreReportTimestamp := true
	restoreReportCaller := true
	// The TimeFormat is set by CreateLogger based on verbose,
	// and SetReportTimestamp(true) will use the existing format.

	// Temporarily change logger settings for capture
	utils.Logger.SetOutput(&buf)
	utils.Logger.SetReportTimestamp(false) // Disable for predictable test output
	utils.Logger.SetReportCaller(false)    // Disable for predictable test output

	defer func() {
		// Restore logger settings
		utils.Logger.SetOutput(os.Stderr) // utils.CreateLogger always uses os.Stderr
		utils.Logger.SetReportTimestamp(restoreReportTimestamp)
		utils.Logger.SetReportCaller(restoreReportCaller)
		// If utils.CreateLogger was called with true, it sets a specific time format.
		// SetReportTimestamp(true) should reuse it. If CreateLogger(false) was called,
		// then timeFormat was "", and SetReportTimestamp(true) alone might not bring back
		// a specific format if one was desired. However, for test log capturing,
		// this restoration is generally sufficient.
		// If a very specific TimeFormat needs restoration, and CreateLogger's state is complex,
		// one might need to call utils.CreateLogger(true) again in the defer,
		// but that might have other side effects if CreateLogger does more than just set these.
		// For now, this simpler restoration is cleaner.
	}()

	fn() // Execute the function that logs
	return buf.String()
}

func TestNewClient_WithToken(t *testing.T) {
	utils.CreateLogger(true)
	t.Setenv("ACTLOCK_DEBUG", "true")
	t.Setenv("GITHUB_TOKEN", "fake-test-token")
	// os.Getenv("GITHUB_TOKEN")

	ctx := context.Background()
	var client *github.Client
	var err error

	logMsgs := captureLogOutput(func() {
		client, err = githubclient.NewClient(ctx)
	})

	require.NoError(t, err)
	require.NotNil(t, client)

	// Check stdout message
	assert.Contains(t, logMsgs, "ℹ️  Could not determine GitHub API authentication status.")

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
	utils.CreateLogger(true)
	t.Setenv("ACTLOCK_DEBUG", "true")
	t.Setenv("GITHUB_TOKEN", "") // Ensure token is unset

	ctx := context.Background()
	var client *github.Client
	var err error

	logMsgs := captureLogOutput(func() {
		client, err = githubclient.NewClient(ctx)
	})

	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Contains(
		t,
		logMsgs,
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
	utils.CreateLogger(true)
	// This output will only be displayed when debugging
	t.Setenv("ACTLOCK_DEBUG", "true")

	envVar := os.Getenv("ACTLOCK_DEBUG")

	log.Printf("ACTLOCK_DEBUG is: %s", envVar)

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
