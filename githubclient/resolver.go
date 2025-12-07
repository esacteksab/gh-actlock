// SPDX-License-Identifier: MIT
package githubclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v80/github"

	"github.com/esacteksab/gh-actlock/utils"
)

// ResolveRefToSHA attempts to find the commit SHA for a given Git ref (tag, branch, or potential SHA).
// It checks in the order:
// 1. If the ref itself is a valid, existing commit SHA.
// 2. If the ref matches an existing Git tag (handling lightweight and annotated tags).
// 3. If the ref matches an existing Git branch.
//
// - ctx: The context for the API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - owner: The owner (user or organization) of the GitHub repository (e.g., "actions").
// - repo: The name of the GitHub repository (e.g., "checkout").
// - ref: The Git reference string to resolve (e.g., "v4", "main", "abcdef").
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
	// verifyCommitSHA will return the SHA and true if it's a valid, existing commit SHA.
	if sha, isCommit, err := verifyCommitSHA(ctx, client, owner, repo, ref); err != nil {
		// Log non-critical errors during verification (e.g. network issues during check, but not 404).
		// This doesn't stop the process - we'll continue to check tags/branches.
		utils.Logger.Errorf(
			"Warning: Error verifying potential SHA '%s' for %s/%s: %v. Proceeding to check tags/branches.",
			ref,
			owner,
			repo,
			err,
		)
		// Error here means verification failed, not necessarily that it's *not* a commit.
		// We'll continue to check tags/branches just in case.
	} else if isCommit {
		// If verifyCommitSHA confirmed this is a valid commit SHA that exists in the repo.
		utils.Logger.Debugf("Ref '%s' is already a valid commit SHA.", ref) // Optional verbose log
		return sha, nil                                                     // Return the verified SHA.
	}

	// 2. If it wasn't a verified commit SHA, try resolving it as a Git tag.
	// resolveTagToSHA returns the resolved SHA, a boolean indicating if a tag was found,
	// the associated HTTP response, and an error.
	if sha, found, resp, err := resolveTagToSHA(ctx, client, owner, repo, ref); err != nil {
		// Log errors unless it's a simple "not found" (HTTP 404 from the initial GetRef call), which is expected when checking.
		if !isNotFoundError(err, resp) { // Use the resp returned by resolveTagToSHA
			utils.Logger.Errorf(
				"Warning: Error checking tag '%s' for %s/%s: %v",
				ref,
				owner,
				repo,
				err,
			)
		}
		// Continue even if there was an error checking the tag, unless it's critical and returned found=true with an error
	} else if found {
		// If a tag with this name was found and resolved to a SHA.
		utils.Logger.Debugf("  Resolved ref '%s' via tag to SHA: %s", ref, sha[:8]) // Log resolved SHA (truncated)
		return sha, nil                                                             // Return the resolved SHA.
	}

	// 3. If it wasn't a tag, try resolving it as a branch.
	// resolveBranchToSHA returns the resolved SHA, a boolean indicating if a branch was found,
	// the associated HTTP response, and an error.
	if sha, found, resp, err := resolveBranchToSHA(ctx, client, owner, repo, ref); err != nil {
		// Log errors unless it's a simple "not found" (HTTP 404), which is expected when checking.
		if !isNotFoundError(err, resp) { // Use the resp returned by resolveBranchToSHA
			utils.Logger.Errorf(
				"Warning: Error checking branch '%s' for %s/%s: %v",
				ref,
				owner,
				repo,
				err,
			)
		}
		// Continue even if there was an error checking the branch
	} else if found {
		// If a branch with this name was found and resolved to a SHA.
		utils.Logger.Debugf("  Resolved ref '%s' via branch to SHA: %s", ref, sha[:8]) // Log resolved SHA (truncated)
		return sha, nil                                                                // Return the resolved SHA.
	}

	// 4. If we've tried all options (commit SHA check, tag lookup, branch lookup)
	// and nothing matched or resolved successfully, return a "not found" error.
	return "", fmt.Errorf("reference '%s' not found as a tag or branch in %s/%s", ref, owner, repo)
}

// verifyCommitSHA checks if a given string 'ref' is formatted like a SHA-1 and
// verifies if it corresponds to an actual, existing commit in the repository.
//
// - ctx: The context for the API calls.
// - client: The initialized GitHub client.
// - owner: The owner of the GitHub repository.
// - repo: The name of the GitHub repository.
// - ref: The string to verify as a potential commit SHA.
// Returns:
//   - sha: The verified SHA (same as ref if valid), empty string otherwise.
//   - isCommit: A boolean indicating if the string is a valid, existing commit SHA.
//   - err: An error if a critical API call failed (excluding 404 Not Found).
func verifyCommitSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (string, bool, error) {
	// A valid SHA must be exactly 40 characters long and contain only hexadecimal digits.
	if len(ref) != SHALength || !IsHexString(ref) {
		// If the format is wrong, it's definitely not a full SHA. No API call needed.
		return "", false, nil
	}

	// Attempt to get the commit details for this potential SHA using the GitHub API.
	// If the API call succeeds (HTTP 200), it's a valid, existing commit SHA.
	// If the API returns 404 Not Found, it's a valid format but doesn't exist in this repo.
	// Other errors (network, rate limit etc.) should be propagated.
	_, resp, err := client.Git.GetCommit(
		ctx,
		owner,
		repo,
		ref,
	) // Keep `resp` to check status code on error
	if err != nil {
		// Check if the error is a GitHub API error indicating 'Not Found' (404).
		// isNotFoundError checks both the error type and the response status code.
		if isNotFoundError(err, resp) {
			// It looked like a SHA format-wise, but this SHA doesn't exist in the repo.
			return "", false, nil
		}
		// For any other error (network, auth, rate limit, etc.), return the error.
		return "", false, fmt.Errorf(
			"failed to verify commit SHA '%s' for %s/%s: %w",
			ref,
			owner,
			repo,
			err,
		)
	}

	// If there was no error, the API call was successful, meaning the SHA is valid and exists.
	// The GitHub client automatically checks for successful status codes.
	return ref, true, nil
}

// resolveTagToSHA attempts to find a Git tag with the given name and return its associated commit SHA.
// It correctly handles both lightweight tags (pointing directly to a commit) and annotated tags
// (pointing to a tag object, which in turn points to a commit).
//
// - ctx: The context for the API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - owner: The owner of the GitHub repository.
// - repo: The name of the GitHub repository.
// - ref: The potential tag name (e.g., "v4", "latest").
//
// Returns:
//   - sha: The commit SHA the tag ultimately points to.
//   - found: A boolean indicating if a tag with the given name was found.
//   - resp: The GitHub API response object from the last successful or failed call.
//   - err: An error if a critical API call failed during resolution (excluding initial 404).
func resolveTagToSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (sha string, found bool, resp *github.Response, err error) {
	// GitHub API uses "refs/tags/" prefix for tag references.
	refPath := "refs/tags/" + ref

	// 1. Attempt to get the Git Reference object for the tag.
	// This tells us what kind of object the tag name points to (commit or tag object).
	gitRef, respRef, errRef := client.Git.GetRef(ctx, owner, repo, refPath)
	if errRef != nil {
		// If the GetRef call failed:
		// If it's a 404 error, the tag simply doesn't exist. This is not a critical error for the overall process.
		if isNotFoundError(errRef, respRef) {
			return "", false, respRef, nil // Tag not found by GetRef, return found=false.
		}
		// For any other error (network, auth, rate limit), return the error.
		return "", false, respRef, fmt.Errorf(
			"failed to get tag ref '%s' (%s) for %s/%s: %w",
			ref,
			refPath,
			owner,
			repo,
			errRef,
		)
	}

	// If gitRef is nil but no error occurred, it's an unexpected state.
	if gitRef == nil || gitRef.Object == nil || gitRef.Object.SHA == nil {
		return "", false, respRef, fmt.Errorf(
			"tag ref '%s' (%s) for %s/%s found but returned unexpected nil object/SHA",
			ref,
			refPath,
			owner,
			repo,
		)
	}

	// 2. Check the type of object the Git Reference points to.
	switch *gitRef.Object.Type {
	case "commit":
		// This is a lightweight tag. It points directly to a commit.
		utils.Logger.Debugf(
			"  Tag '%s' (%s) is lightweight, pointing directly to commit %s",
			ref,
			refPath,
			(*gitRef.Object.SHA)[:8],
		)
		// The SHA we need is the one in the reference's object.
		return *gitRef.Object.SHA, true, respRef, nil // Return the commit SHA, and the response from GetRef.

	case "tag":
		// This is an annotated tag. It points to a Git Tag object.
		// We need to fetch the Tag object to find the commit it points to.
		tagObjectSHA := *gitRef.Object.SHA
		utils.Logger.Debugf(
			"  Tag '%s' (%s) is annotated, pointing to tag object %s. Fetching tag object...",
			ref,
			refPath,
			tagObjectSHA[:8],
		)

		// 3. Fetch the Git Tag object using the SHA obtained from the reference object.
		gitTag, respTag, errTag := client.Git.GetTag(ctx, owner, repo, tagObjectSHA)
		if errTag != nil {
			// If GetTag fails, this *is* a critical error because the tag object should exist if GetRef said so.
			// If it's a 404 here, it indicates an inconsistency or a cache issue.
			return "", false, respTag, fmt.Errorf(
				"failed to get tag object '%s' for annotated tag '%s': %w",
				tagObjectSHA,
				ref,
				errTag,
			)
		}

		// If gitTag is nil but no error occurred, it's an unexpected state.
		if gitTag == nil || gitTag.Object == nil || gitTag.Object.SHA == nil {
			return "", false, respTag, fmt.Errorf(
				"tag object '%s' for annotated tag '%s' found but returned unexpected nil object/SHA",
				tagObjectSHA,
				ref,
			)
		}

		// 4. Check the type of object the Tag object points to.
		// For actions/workflows, the tag object should point to a commit.
		if *gitTag.Object.Type != "commit" {
			// The tag object points to something unexpected (e.g., another tag object, or a tree/blob).
			// This is not a standard action/workflow tag structure.
			return "", false, respTag, fmt.Errorf(
				"tag object '%s' for annotated tag '%s' points to object type '%s', expected 'commit'",
				tagObjectSHA,
				ref,
				*gitTag.Object.Type,
			)
		}

		// The SHA in the Tag object's Object field is the commit SHA that the tag points to.
		commitSHA := *gitTag.Object.SHA
		utils.Logger.Debugf(
			"  Annotated tag object %s points to commit SHA: %s",
			tagObjectSHA[:8],
			commitSHA[:8],
		)
		// Return the commit SHA and the response from the GetTag call (as it was the last successful/relevant one).
		return commitSHA, true, respTag, nil

	default:
		// The Git Reference object points to something other than a commit or a tag object (e.g., a tree or blob).
		// This is not a standard tag structure.
		return "", false, respRef, fmt.Errorf(
			"tag ref '%s' (%s) for %s/%s points to unexpected object type '%s'",
			ref, refPath, owner, repo, *gitRef.Object.Type)
	}
}

// resolveBranchToSHA attempts to find a Git branch with the given name and return its head commit SHA.
// It uses the GetRef API call for branch references ("refs/heads/").
//
// - ctx: The context for the API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - owner: The owner of the GitHub repository.
// - repo: The name of the GitHub repository.
// - ref: The potential branch name (e.g., "main", "dev").
//
// Returns:
//   - sha: The commit SHA at the head of the branch.
//   - found: A boolean indicating if a branch with the given name was found.
//   - resp: The GitHub API response object from the GetRef call.
//   - err: An error if the API call failed (excluding 404 Not Found).
func resolveBranchToSHA(
	ctx context.Context,
	client *github.Client,
	owner, repo, ref string,
) (sha string, found bool, resp *github.Response, err error) {
	// GitHub API uses "refs/heads/" prefix for branch references.
	refPath := "refs/heads/" + ref

	// Attempt to get the Git Reference object for the branch.
	gitRef, resp, err := client.Git.GetRef(ctx, owner, repo, refPath)
	if err != nil {
		// If the GetRef call failed:
		// If it's a 404 error, the branch simply doesn't exist. Return found=false.
		if isNotFoundError(err, resp) {
			return "", false, resp, nil // Branch not found.
		}
		// For any other error (network, auth, rate limit), return the error.
		return "", false, resp, fmt.Errorf(
			"failed to get branch ref '%s' (%s) for %s/%s: %w",
			ref,
			refPath,
			owner,
			repo,
			err,
		)
	}

	// If gitRef is nil but no error occurred, it's an unexpected state.
	if gitRef == nil || gitRef.Object == nil || gitRef.Object.SHA == nil {
		return "", false, resp, fmt.Errorf(
			"branch ref '%s' (%s) for %s/%s found but returned unexpected nil object/SHA",
			ref,
			refPath,
			owner,
			repo,
		)
	}

	// A branch reference object should point directly to a commit object type.
	if *gitRef.Object.Type != "commit" {
		// The ref object points to something other than a commit.
		// This is not a standard branch structure.
		return "", false, resp, fmt.Errorf(
			"branch ref '%s' (%s) for %s/%s points to unexpected object type '%s', expected 'commit'",
			ref,
			refPath,
			owner,
			repo,
			*gitRef.Object.Type,
		)
	}

	// The SHA in the reference object's Object field is the commit SHA at the head of the branch.
	commitSHA := *gitRef.Object.SHA
	utils.Logger.Debugf(
		"  Resolved branch '%s' (%s) to commit SHA: %s",
		ref,
		refPath,
		commitSHA[:8],
	)
	return commitSHA, true, resp, nil // Return the commit SHA and the response from GetRef.
}

// isNotFoundError is a helper function to check if an error returned by the GitHub
// API client corresponds to an HTTP 404 Not Found error. This is useful for
// distinguishing between expected 'not found' results during lookups (like a tag
// or branch not existing) and actual errors (like rate limits, network issues).
//
// - err: The error returned by a GitHub API call.
// - resp: The *github.Response object associated with the API call.
// Returns: true if the error is a GitHub API ErrorResponse with a 404 status code, false otherwise.
func isNotFoundError(err error, resp *github.Response) bool {
	// First, check if the error is specifically a GitHub API ErrorResponse type.
	// This uses Go's type assertion to check the error type.
	if errResp, ok := err.(*github.ErrorResponse); ok {
		// If it is a GitHub error, we also check that:
		// 1. The response object is not nil (to avoid null pointer exceptions).
		// 2. The HTTP status code is specifically 404 (Not Found).
		// We need both checks because the error type tells us it's a GitHub API error,
		// and the response status code tells us it's a 404 specifically.
		return errResp != nil &&
			resp.StatusCode == http.StatusNotFound // Check the response status code
	}
	// If the error is not a GitHub API ErrorResponse, it's not a 404 we handle this way.
	return false
}
