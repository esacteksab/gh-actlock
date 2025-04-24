// SPDX-License-Identifier: MIT

package githubclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http" // Needed for comparing HTTP status codes

	"github.com/google/go-github/v71/github" // GitHub API client library
)

// ResolveRefToSHA attempts to find the commit SHA for a given Git ref (tag, branch, or potential SHA).
// It checks in the order:
// 1. If the ref itself is a valid, existing commit SHA.
// 2. If the ref matches an existing Git tag.
// 3. If the ref matches an existing Git branch.
//
// -ctx: The context for the API calls, allows for cancellation/timeouts.
// -client: The initialized GitHub client for making API requests.
// -owner: The owner (user or organization) of the GitHub repository (e.g., "actions").
// -repo: The name of the GitHub repository (e.g., "checkout").
// -ref: The Git reference string to resolve (e.g., "v4", "main", "abcdef").
// Returns: The full 40-character SHA-1 hash as a string if resolved, or an empty string and an error if not found or a critical error occurs.
func ResolveRefToSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (string, error) {
	// Basic validation of input parameters.
	if client == nil {
		return "", errors.New("github client is nil")
	}
	if owner == "" || repo == "" || ref == "" {
		return "", errors.New("owner, repo, and ref must not be empty")
	}

	// 1. First, check if the provided 'ref' string is already a valid commit SHA.
	// This avoids unnecessary API calls if the reference is already a commit hash.
	if sha, isCommit, err := verifyCommitSHA(ctx, client, owner, repo, ref); err != nil {
		// Log non-critical errors during verification (e.g., network issues during check).
		// This doesn't stop the process - we'll continue to check tags/branches.
		log.Printf(
			"Warning: Error verifying potential SHA '%s': %v. Proceeding to check tags/branches.",
			ref,
			err,
		)
		// Error here means verification failed, not necessarily that it's *not* a commit.
		// We'll continue to check tags/branches just in case.
	} else if isCommit {
		// If verifyCommitSHA confirmed this is a valid commit SHA that exists in the repo.
		// log.Printf("Ref '%s' is already a valid commit SHA.", ref) // Optional verbose log
		return sha, nil // Return the verified SHA.
	}

	// 2. If it wasn't a verified commit SHA, try resolving it as a Git tag.
	if sha, found, resp, err := resolveTagToSHA(ctx, client, owner, repo, ref); err != nil {
		// Log errors unless it's a simple "not found" (HTTP 404), which is expected when checking.
		if !isNotFoundError(err, resp) {
			log.Printf("Warning: Error checking tag '%s': %v", ref, err)
		}
	} else if found {
		// If a tag with this name was found.
		log.Printf("  Resolved ref '%s' via tag to SHA: %s", ref, sha)
		return sha, nil // Return the SHA associated with the tag.
	}

	// 3. If it wasn't a tag, try resolving it as a branch.
	if sha, found, resp, err := resolveBranchToSHA(ctx, client, owner, repo, ref); err != nil {
		// Log errors unless it's a simple "not found" (HTTP 404).
		if !isNotFoundError(err, resp) {
			log.Printf("Warning: Error checking branch '%s': %v", ref, err)
		}
	} else if found {
		// If a branch with this name was found.
		log.Printf("  Resolved ref '%s' via branch to SHA: %s", ref, sha)
		return sha, nil // Return the SHA associated with the branch head.
	}

	// 4. If we've tried all options and nothing matched, return an error
	return "", fmt.Errorf("reference '%s' not found as a tag or branch in %s/%s", ref, owner, repo)
}

// verifyCommitSHA checks if a given string 'ref' is a valid and existing commit SHA in the repository.
//
// -ctx: The context for the API calls, allows for cancellation/timeouts.
// -client: The initialized GitHub client for making API requests.
// -owner: The owner (user or organization) of the GitHub repository.
// -repo: The name of the GitHub repository.
// -ref: The string to verify as a potential commit SHA.
// Returns: The verified SHA (same as ref if valid), a boolean indicating if it is a valid commit, and an error.
func verifyCommitSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (string, bool, error) {
	// A valid SHA must be exactly 40 characters long (SHALength constant) and contain only hexadecimal digits (0-9, a-f).
	// IsHexString checks if all characters in the string are valid hex digits.
	if len(ref) != SHALength || !IsHexString(ref) {
		return ref, false, nil // Not even the right format for a SHA, so it's not a commit.
	}

	// Attempt to get the commit details for this potential SHA using the GitHub API.
	// The API call returns three values:
	// 1. The commit object (which we don't need here, so we use _)
	// 2. The HTTP response object (which we need to check for 404 status specifically)
	// 3. An error if the API call failed
	_, resp, err := client.Git.GetCommit(ctx, owner, repo, ref)
	if err != nil {
		// Check if the error indicates "Not Found" (404 HTTP status code).
		// This is an expected error when checking if a SHA exists.
		if isNotFoundError(err, resp) {
			return ref, false, nil // It looked like a SHA format-wise, but this SHA doesn't exist in the repo.
		}
		// For any other error (network issues, permission denied, etc.), return the error.
		return ref, false, fmt.Errorf("failed to verify commit SHA '%s': %w", ref, err)
	}

	// If there was no error and the response was successful, it's a valid commit SHA.
	// The GitHub client automatically checks for successful status codes.
	return ref, true, nil
}

// resolveToSHA attempts to find a Git reference of the specified type and return its associated commit SHA.
//
// -ctx: The context for the API calls, allows for cancellation/timeouts.
// -client: The initialized GitHub client for making API requests.
// -owner: The owner (user or organization) of the GitHub repository.
// -repo: The name of the GitHub repository.
// -refType: The type of reference ("tags" or "heads").
// -ref: The reference name (e.g., "v4" for a tag or "main" for a branch).
// Returns:
//   - sha: The commit SHA the reference points to
//   - found: A boolean indicating if the reference was found
//   - resp: The GitHub API response object
//   - err: An error if one occurred
func resolveToSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, refType, ref string,
) (sha string, found bool, resp *github.Response, err error) {
	// GitHub API requires "refs/TYPE/" prefix for references.
	refPath := fmt.Sprintf("refs/%s/%s", refType, ref)

	gitRef, resp, err := client.Git.GetRef(ctx, owner, repo, refPath)
	if err != nil {
		// Check if this is a "Not Found" error (HTTP 404), which is common during lookups.
		if isNotFoundError(err, resp) {
			return "", false, resp, nil // Reference simply doesn't exist - not an error condition.
		}
		// For other errors (rate limits, permission issues, etc.), return as an actual error.
		return "", false, resp, fmt.Errorf("failed to get %s ref '%s': %w", refType, ref, err)
	}

	// If we successfully found the reference, extract the SHA from the reference object.
	if gitRef != nil && gitRef.Object != nil && gitRef.Object.SHA != nil {
		// The SHA is stored in gitRef.Object.SHA (a pointer to a string)
		return *gitRef.Object.SHA, true, resp, nil
	}

	// If the reference object was found but had missing data (unusual case)
	return "", false, resp, fmt.Errorf(
		"%s ref '%s' for %s/%s found but object SHA is nil",
		refType,
		ref,
		owner,
		repo,
	)
}

// resolveTagToSHA attempts to find a Git tag with the given name and return its associated commit SHA.
//
// -ctx: The context for the API calls, allows for cancellation/timeouts.
// -client: The initialized GitHub client for making API requests.
// -owner: The owner (user or organization) of the GitHub repository.
// -repo: The name of the GitHub repository.
// -ref: The potential tag name (e.g., "v4").
// Returns:
//   - sha: The commit SHA the tag points to
//   - found: A boolean indicating if a tag was found
//   - resp: The GitHub API response object
//   - err: An error if one occurred
func resolveTagToSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (sha string, found bool, resp *github.Response, err error) {
	return resolveToSHA(ctx, client, owner, repo, "tags", ref)
}

// resolveBranchToSHA attempts to find a Git branch with the given name and return its head commit SHA.
//
// -ctx: The context for the API calls, allows for cancellation/timeouts.
// -client: The initialized GitHub client for making API requests.
// -owner: The owner (user or organization) of the GitHub repository.
// -repo: The name of the GitHub repository.
// -ref: The potential branch name (e.g., "main").
// Returns:
//   - sha: The commit SHA at the head of the branch
//   - found: A boolean indicating if a branch was found
//   - resp: The GitHub API response object
//   - err: An error if one occurred
func resolveBranchToSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (sha string, found bool, resp *github.Response, err error) {
	return resolveToSHA(ctx, client, owner, repo, "heads", ref)
}

// isNotFoundError is a helper function to check if an error is a GitHub API error
// specifically indicating a "Not Found" (HTTP 404) response.
// This distinction is important because "not found" is often an expected result during lookups,
// while other API errors (rate limits, network issues) are more serious.
//
// -err: The error to check.
// -resp: The GitHub API response object associated with the error.
// Returns: true if the error is a GitHub API error with a 404 status code, false otherwise.
func isNotFoundError(err error, resp *github.Response) bool {
	// First, check if the error is specifically a GitHub API ErrorResponse type.
	// This uses Go's type assertion (err.(*github.ErrorResponse)) to check the error type.
	if _, ok := err.(*github.ErrorResponse); ok {
		// If it is a GitHub error, we also check that:
		// 1. The response object is not nil (to avoid null pointer exceptions)
		// 2. The HTTP status code is specifically 404 (Not Found)
		//
		// We need both checks because:
		// - The error tells us it's a GitHub API error
		// - The response confirms the specific type of API error (404 Not Found)
		return resp != nil && resp.StatusCode == http.StatusNotFound
	}
	// If the error is not a GitHub API ErrorResponse, it's not a 404 we handle this way.
	return false
}
