// SPDX-License-Identifier: MIT

package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/log"
)

var Logger *log.Logger

// Maximum filename length supported by most filesystems
const maxFilenameLength = 255

// ValidateFilePath validates a file path to ensure it's safe for filesystem operations.
// It performs multiple security checks including path traversal prevention, character validation,
// and ensuring the path contains only a filename without directory components.
//
// -path: The file path string to validate.
// Returns: The cleaned and validated file path, and an error if validation fails.
func ValidateFilePath(path string) (string, error) {
	// 1. Check for empty path
	if path == "" {
		return path, errors.New("invalid file path: filename cannot be empty")
	}

	// 2. Basic cleaning (removes dots, extra slashes, normalizes separators)
	// filepath.Clean handles tasks like:
	// - Converting "a/./b" to "a/b"
	// - Converting "a//b" to "a/b"
	// - Converting "a/b/" to "a/b"
	cleanedPath := filepath.Clean(path)

	// --- Security Checks ---

	// 3. Check for remaining path traversal components after cleaning
	// Path traversal is when ".." is used to navigate to parent directories,
	// which could potentially allow access to files outside intended scope.
	// Examples:
	//    filepath.Clean("a/../../b") -> "../b"
	//    filepath.Clean("../a") -> "../a"
	//    We must ensure no ".." component exists anywhere in the path.
	parts := strings.Split(cleanedPath, string(filepath.Separator))
	if slices.Contains(parts, "..") {
		// Return original path and error
		return path, fmt.Errorf("invalid file path %q: contains '..'", path)
	}

	// 4. Check if the cleaned path contains any directory separators.
	// This catches absolute paths (`/` or `C:\`) and nested paths (`a/b`).
	// We compare the cleaned path with its base name. If they differ, it means
	// the path contained separators.
	// Examples:
	//    filepath.Base("/a/b") -> "b"
	//    filepath.Base("a/b") -> "b"
	//    filepath.Base("a") -> "a"
	if cleanedPath != filepath.Base(cleanedPath) {
		// Return original path and error
		return path, fmt.Errorf(
			"invalid file path %q: must be a filename only, no slashes or traversal",
			path,
		)
	}

	// --- Standard Checks (on the final candidate filename: cleanedPath) ---

	// 5. Check filename length against maximum allowed
	// Many filesystems have limits on filename length, typically 255 characters
	if len(cleanedPath) > maxFilenameLength {
		// Return original path and error
		return path, fmt.Errorf(
			"invalid file path: filename %q (cleaned to %q) exceeds maximum length of %d",
			path,
			cleanedPath,
			maxFilenameLength,
		)
	}

	// 6. Check for null bytes which can cause security issues
	// Null bytes can cause string termination issues in some programming languages
	// and libraries, potentially leading to security vulnerabilities
	if strings.ContainsRune(cleanedPath, '\x00') {
		// Return original path and error
		return path, fmt.Errorf(
			"invalid file path: filename %q (cleaned to %q) contains null byte",
			path,
			cleanedPath,
		)
	}

	// 7. Check for other invalid characters
	// Different operating systems have different restrictions on valid filename characters
	// This is a common set of characters that are problematic across many systems
	invalidChars := ";|$\"()`" // Semi-colon, pipe, dollar, quotes, parentheses, backtick
	if strings.ContainsAny(cleanedPath, invalidChars) {
		// Return original path and error
		return path, fmt.Errorf(
			"invalid file path: filename %q (cleaned to %q) contains invalid characters",
			path,
			cleanedPath,
		)
	}

	// If all checks pass, return the CLEANED path and nil error
	// The cleaned path is guaranteed to be just a base filename at this point.
	return cleanedPath, nil
}

// ValidateWorkflowFilePath validates a workflow file path to ensure it is accessible
// within the project root and doesn't contain potentially unsafe path components.
//
// -filePath: The path to the workflow file to validate.
// Returns: An error if the path is invalid, traverses outside project root, or cannot be resolved.
func ValidateWorkflowFilePath(filePath string) error {
	// Check for explicit '..' components in the raw path string.
	// This is a defense-in-depth measure against path traversal attacks,
	// which could potentially access files outside the intended directory.
	if strings.Contains(filePath, "..") {
		return fmt.Errorf("workflow path %q contains '..'", filePath)
	}

	// Resolve the absolute path to eliminate any relative components and
	// normalize the path for comparison with the project root.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("could not get absolute path for %q: %w", filePath, err)
	}

	// Get current working directory, which is assumed to be the project root.
	// This will be used to verify that the file exists within the project boundaries.
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get working directory: %w", err)
	}

	// Check if the file is within the working directory or is the working directory itself.
	// The file must either:
	// 1. Have a path that begins with the working directory path followed by a separator, or
	// 2. Be the working directory itself (though this is an edge case)
	if !strings.HasPrefix(absPath, wd+string(filepath.Separator)) && absPath != wd {
		return fmt.Errorf("workflow path %q resolves outside project root %q", filePath, wd)
	}

	// If all checks pass, the path is considered valid
	return nil
}

func LogRateLimitStatus(limitType string) {
	switch limitType {
	case "authenticated":
		Logger.Print("🔧  Authenticated GitHub API access in effect.")
	case "unauthenticated":
		Logger.Print(
			"⚠️  Unauthenticated GitHub API access in effect (lower rate limit).",
		)
	default:
		Logger.Print("ℹ️  Could not determine GitHub API authentication status.")
	}
}
