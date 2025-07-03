// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v72/github"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/esacteksab/gh-actlock/githubclient"
	"github.com/esacteksab/gh-actlock/parser"
	"github.com/esacteksab/gh-actlock/utils"
)

// Constants for GitHub workflow directory structure.
const (
	ghDir = ".github"   // The root directory for GitHub related files
	wfDir = "workflows" // The subdirectory for workflow YAML files
)

// Variables to hold build information, populated at build time.
var (
	Version string // Application version
	Date    string // Build date
	Commit  string // Git commit hash
	BuiltBy string // Builder identifier
	Update  bool   // Whether to update SHAs
	Clear   bool   // Whether to clear cache
)

// init is automatically run before the main function.
// It sets the version information for the root command using build-time variables.
func init() {
	// BuildVersion utility formats the version string.
	rootCmd.Version = utils.BuildVersion(Version, Commit, Date, BuiltBy)
	// SetVersionTemplate customizes how the version is printed.
	rootCmd.SetVersionTemplate(`{{printf "Version %s" .Version}}`)
	rootCmd.Flags().BoolVarP(&Update, "update", "u", false, "update SHAs")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
//
// Returns: Does not return a value, but exits the program with status code 1 if an error occurs.
func Execute() {
	// Execute the root command. If an error occurs, print it to stderr and exit.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// rootCmd represents the base command when called without any subcommands.
// It is the entry point for the actlock application.
var rootCmd = &cobra.Command{
	Use:   "actlock [arg]",                                              // How the command is invoked
	Short: "actlock locks GitHub Actions to SHAs for greater security.", // Short description
	// SilenceUsage prevents usage being printed on error (errors are handled explicitly).
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	// Run defines the main logic of the command when it's executed.
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			fmt.Println("Echo: ", args[0])
		}

		if Update {
			log.Printf("Running in update mode: will update actions to latest versions")
		} else {
			log.Printf("Running in pin mode: will pin actions to specific SHAs")
		}
		// context.Background() is the default context, suitable for the top-level command.
		ctx := context.Background()

		// Initialize the GitHub client using the dedicated package.
		client, err := githubclient.NewClient(ctx)
		if err != nil {
			// Log a fatal error and exit if the client cannot be initialized.
			log.Fatalf("Failed to initialize GitHub client: %v", err)
		}

		// Check the current GitHub API rate limit.
		limitType := githubclient.CheckRateLimit(ctx, client)
		utils.LogRateLimitStatus(limitType)

		// Construct the path to the workflows directory.
		workflowsDir := filepath.Join(ghDir, wfDir)
		// Read the directory entries.
		workflows, err := os.ReadDir(workflowsDir)
		if err != nil {
			// If the directory doesn't exist, provide a specific error message.
			if os.IsNotExist(err) {
				log.Fatalf("Workflows directory not found: %s", workflowsDir)
			}
			// For any other error reading the directory, log a fatal error.
			log.Fatalf("Error reading workflows directory '%s': %v", workflowsDir, err)
		}

		// If no files are found in the directory, print a message and exit.
		if len(workflows) == 0 {
			log.Printf("No workflow files found in %s", workflowsDir)
			return
		}

		log.Printf("Found %d potential workflow files in %s", len(workflows), workflowsDir)
		totalUpdates := 0

		// Iterate through each entry found in the workflows directory.
		for _, wf := range workflows {
			// Skip directories and files starting with '.' (like .gitignore).
			if wf.IsDir() || strings.HasPrefix(wf.Name(), ".") {
				continue
			}
			// Only process files with .yml or .yaml extensions (case-insensitive comparison isn't strictly needed here based on typical filenames).
			if !strings.HasSuffix(wf.Name(), ".yml") && !strings.HasSuffix(wf.Name(), ".yaml") {
				log.Printf("Skipping non-YAML file: %s", wf.Name())
				continue
			}

			// Construct the full path to the workflow file.
			filePath := filepath.Join(workflowsDir, wf.Name())
			log.Printf("Processing workflow: %s", filePath)

			// Call the function to update SHAs within this specific workflow file.
			updated, err := UpdateWorkflowActionSHAs(ctx, client, filePath)
			if err != nil {
				// Log errors related to processing a single file but continue to the next.
				log.Printf("❌  Failed to process %s: %v", filePath, err)
			} else if updated > 0 {
				// Log success if updates were made.
				log.Printf("✅  Updated %d action(s) in %s", updated, filePath)
				totalUpdates += updated
			} else {
				// Log if no updates were needed for the file.
				log.Printf("ℹ️  No actions needed updating in %s", filePath)
			}
		}
		// Final summary of total updates made across all files.
		log.Printf("Finished processing. Total actions updated across all files: %d", totalUpdates)
	},
}

// findUpdatesInNodes recursively searches a YAML node tree for 'uses:' keys,
// processes their values, and populates a map with line numbers requiring updates.
//
// - ctx: The context for API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for resolving SHAs.
// - node: The current YAML node being processed.
// - updates: A map where line numbers are keys and the desired new 'uses:' string values are the values.
// - updatesMade: A pointer to an integer counter tracking the total number of updates found.
// Returns: An error if a critical issue occurs during traversal or processing, otherwise nil.
func findUpdatesInNodes(
	ctx context.Context,
	client *github.Client,
	node *yaml.Node,
	updates map[int]string,
	updatesMade *int,
) error {
	// Different processing based on the type of YAML node
	switch node.Kind {
	case yaml.DocumentNode:
		// A document node represents the root of a YAML document. Iterate its content.
		for _, contentNode := range node.Content {
			// Recursively call findUpdatesInNodes on the content node.
			if err := findUpdatesInNodes(ctx, client, contentNode, updates, updatesMade); err != nil {
				return err // Propagate errors from deeper levels.
			}
		}
	case yaml.MappingNode:
		// A mapping node represents key-value pairs (like a dictionary).
		// Content is a slice of nodes: [key1, value1, key2, value2, ...].
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]     // The key node (e.g., 'uses')
			valueNode := node.Content[i+1] // The value node (e.g., 'actions/checkout@v4')

			// Check if the current key is 'uses' and the value is a simple scalar (a single string).
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "uses" &&
				valueNode.Kind == yaml.ScalarNode {
				// If it's a 'uses:' entry, handle its specific value.
				err := handleUsesValue(ctx, client, valueNode, updates, updatesMade)
				if err != nil {
					// Log the error from handling the 'uses' value but continue processing other parts of the file.
					log.Printf(
						"Error processing 'uses' value on line %d: %v. Skipping this entry.",
						valueNode.Line,
						err,
					)
					// The decision to continue or halt on error here is a design choice.
					// Returning the error (`return err`) would stop processing the current file.
					// Continuing (`continue`) processes other key-value pairs in the same mapping.
					continue // Continue to the next key-value pair in the mapping.
				}
			} else {
				// If the key is not 'uses' or the value is not a scalar (could be a map or list),
				// recursively check the value node for nested 'uses' entries.
				if err := findUpdatesInNodes(ctx, client, valueNode, updates, updatesMade); err != nil {
					return err // Propagate errors from deeper levels.
				}
			}
		}
	case yaml.SequenceNode:
		// A sequence node represents a list (e.g., a list of steps).
		// Iterate through each item in the sequence.
		for _, itemNode := range node.Content {
			// Recursively call findUpdatesInNodes on each item.
			if err := findUpdatesInNodes(ctx, client, itemNode, updates, updatesMade); err != nil {
				return err // Propagate errors from deeper levels.
			}
		}
		// Scalar nodes (simple values) and Alias nodes do not contain nested 'uses' entries,
		// so no recursive call is needed for those kinds.
	}
	return nil // Return nil if traversal of this node and its children completes without a critical error.
}

// handleUsesValue processes a single YAML node representing the value of a 'uses:' key.
// It parses the action reference, resolves the SHA, and adds an entry to the updates map if necessary.
//
// - ctx: The context for API calls.
// - client: The initialized GitHub client.
// - valueNode: The YAML scalar node containing the action string (e.g., "actions/checkout@v4").
// - updates: The map to store line number -> new 'uses:' string mappings.
// - updatesMade: A pointer to an integer counter to increment if an update is added.
// Returns: An error if a significant issue occurs during SHA resolution, otherwise nil.
func handleUsesValue(
	ctx context.Context,
	client *github.Client,
	valueNode *yaml.Node,
	updates map[int]string,
	updatesMade *int,
) error {
	usesValue := valueNode.Value // Get the string value from the node
	lineNum := valueNode.Line    // Get the original line number of this value

	// Check if we have already identified an update for this specific line number
	// This can happen if an alias points to a node containing 'uses', though rare
	// It's a safety check to prevent duplicate processing of the same line
	if _, exists := updates[lineNum]; exists {
		return nil // This line is already scheduled for an update, skip reprocessing
	}

	// Use the parser package to break down the 'uses' string (e.g. owner/repo/action@ref)
	action, err := parser.ParseActionReference(usesValue)
	if err != nil {
		// If parsing fails, log a warning and skip this action reference
		// This is not a fatal error for the entire file
		log.Printf(
			"⚠️ Skipping 'uses: %s' on line %d due to parsing error: %v",
			usesValue,
			lineNum,
			err,
		)
		return nil // Indicate that this specific 'uses' value processing failed non-fatally
	}

	// We are only interested in pinning standard GitHub actions referenced as owner/repo/action@ref.
	// Skip if it's not a 'github' type action (e.g., 'docker://...'), or if any required part is missing.
	// Optionally uncomment the log below for more verbose output on skipped items.
	// log.Printf("Skipping non-GitHub action or incomplete reference: %s", usesValue)
	if action.Type != "github" || action.Name == "" || action.Repo == "" {
		return nil
	}

	// Check if the ref is already a full SHA
	isSHA := len(action.Ref) == githubclient.SHALength && githubclient.IsHexString(action.Ref)

	// Check if it's likely a reusable workflow
	isWorkflow := strings.Contains(action.Repo, ".yml") || strings.Contains(action.Repo, ".yaml")

	// Delegate to the appropriate handler
	if isWorkflow {
		return handleWorkflowReference(
			ctx,
			client,
			action,
			usesValue,
			lineNum,
			updates,
			updatesMade,
			isSHA,
		)
	}
	return handleActionReference(
		ctx,
		client,
		action,
		usesValue,
		lineNum,
		updates,
		updatesMade,
		isSHA,
	)
}

// handleWorkflowReference processes a reusable workflow reference and either updates it to the latest version
// or pins it to a specific SHA based on the Update flag. This function handles the specific complexities
// of reusable workflows, which may include subpaths within repositories.
//
// - ctx: The context for API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - action: The parsed workflow action containing owner, repo, and reference details.
// - usesValue: The original workflow reference string from the workflow file for logging.
// - lineNum: The line number in the workflow file where this workflow reference appears.
// - updates: A map to store line numbers and their replacement strings.
// - updatesMade: A pointer to an integer counter that tracks the number of updates.
// - isSHA: A boolean indicating if the current reference is already a full SHA.
//
// Returns: An error if a critical operation fails, otherwise nil.
func handleWorkflowReference(
	ctx context.Context,
	client *github.Client,
	action parser.WorkflowAction, // Use the specific type from parser
	usesValue string, // Original value for logging/context
	lineNum int,
	updates map[int]string,
	updatesMade *int,
	isSHA bool,
) error {
	owner := action.Name     // Repository owner (user or organization)
	repoField := action.Repo // Repository name potentially with subpath (e.g., "repo/path/to/workflow.yml")
	ref := action.Ref        // Current reference (tag, branch, or SHA)

	// Extract repository name for API calls
	// For reusable workflows, the repo field might contain a path to the workflow file
	// We need to split at the first slash to get just the repo name for API calls
	repoParts := strings.SplitN(repoField, "/", 2) //nolint:mnd
	repoNameForAPI := repoParts[0]                 // Just the repository name without subpath

	// Validate that we were able to extract a repository name
	if repoNameForAPI == "" {
		log.Printf(
			"❌ Could not extract repository name from '%s' for workflow on line %d. Skipping.",
			repoField,
			lineNum,
		)
		return nil // Continue processing other references
	}

	// Construct the full path for the 'uses' string (owner/repo/path)
	// This is the complete reference as it appears in the workflow file
	fullPathForUses := fmt.Sprintf("%s/%s", owner, repoField)

	// --- Workflow Update Mode ---
	// When Update is true, we're finding the latest version and updating all references
	if Update {
		log.Printf("🔍 Finding latest version for workflow: %s (repo: %s/%s) (line %d)",
			fullPathForUses, owner, repoNameForAPI, lineNum)

		// Get the latest reference and its commit SHA for the repository
		latestRef, commitSHA, err := githubclient.GetLatestActionRef(
			ctx,
			client,
			owner,
			repoNameForAPI,
		)
		if err != nil || commitSHA == "" || latestRef == "" {
			// Log an error if latest version discovery fails
			log.Printf(
				"❌ Error finding latest ref/SHA for workflow repo %s/%s: %v. Skipping update for line %d.",
				owner,
				repoNameForAPI,
				err,
				lineNum,
			)
			return nil // Continue processing other references
		}

		// Create the new workflow reference string with SHA + comment
		newUsesValue := fmt.Sprintf("%s@%s #%s", fullPathForUses, commitSHA, latestRef)

		// Log the update details
		log.Printf(
			"  Updating workflow %s to SHA %s (latest ref: %s)",
			fullPathForUses,
			commitSHA[:8], // Show only first 8 chars of SHA for readability
			latestRef,
		)

		// Check if the workflow is already up-to-date
		if isSHA && ref == commitSHA {
			// If current reference is already the latest SHA, no update needed
			log.Printf(
				"  Workflow %s already up-to-date with SHA %s (latest ref: %s). No change needed.",
				fullPathForUses,
				commitSHA[:8],
				latestRef,
			)
		} else {
			// Store the update in the map and increment counter
			updates[lineNum] = newUsesValue
			*updatesMade++
		}

		return nil // Successfully processed workflow in update mode

		// --- Workflow Pinning Mode ---
		// When Update is false, we're pinning existing references to their current SHA
	} else {
		// If the reference is already a SHA, no need to pin it
		if isSHA {
			log.Printf("ℹ️  Workflow '%s' on line %d already pinned to SHA: %s", usesValue, lineNum, ref)
			return nil // Already pinned, no update needed
		}

		// Resolve the branch/ref to use (handles empty refs by finding default branch)
		branchName, originalRefForComment, err := resolveWorkflowRef(ctx, client, owner, repoNameForAPI, ref, fullPathForUses)
		if err != nil {
			// Log an error if we can't resolve the reference
			log.Printf("❌  Skipping pin for workflow '%s' on line %d: %v", usesValue, lineNum, err)
			return nil // Continue processing other references
		}

		// Log the pinning operation
		log.Printf("🔍  Pinning workflow: %s@%s (line %d) (repo: %s)", fullPathForUses, branchName, lineNum, repoNameForAPI)

		// Resolve the branch/ref to its commit SHA
		commitSHA, err := githubclient.ResolveRefToSHA(ctx, client, owner, repoNameForAPI, branchName)
		if err != nil || commitSHA == "" {
			// Log an error if we can't resolve the SHA
			log.Printf("❌  Error resolving ref '%s' to SHA for workflow %s/%s: %v. Skipping update for line %d.",
				branchName, owner, repoNameForAPI, err, lineNum)
			return nil // Continue processing other references
		}

		// Create the new workflow reference string with SHA + comment
		newUsesValue := fmt.Sprintf("%s@%s #%s", fullPathForUses, commitSHA, originalRefForComment)
		log.Printf("  Pinned workflow %s@%s to SHA %s", fullPathForUses, originalRefForComment, commitSHA[:8])

		// Store the update in the map and increment counter
		updates[lineNum] = newUsesValue
		*updatesMade++

		return nil // Successfully processed workflow in pinning mode
	}
}

// handleActionReference processes a GitHub Action reference and either updates it to the latest version
// or pins it to a specific SHA based on the Update flag. This function handles the core logic
// of determining what changes to make to action references in workflow files.
//
// - ctx: The context for API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - action: The parsed workflow action containing owner, repo, and reference details.
// - usesValue: The original action reference string from the workflow file for logging.
// - lineNum: The line number in the workflow file where this action reference appears.
// - updates: A map to store line numbers and their replacement strings.
// - updatesMade: A pointer to an integer counter that tracks the number of updates.
// - isSHA: A boolean indicating if the current reference is already a full SHA.
//
// Returns: An error if a critical operation fails, otherwise nil.
func handleActionReference(
	ctx context.Context,
	client *github.Client,
	action parser.WorkflowAction, // Use the specific type from parser
	usesValue string, // Original value for logging/context
	lineNum int,
	updates map[int]string,
	updatesMade *int,
	isSHA bool,
) error {
	owner := action.Name    // Repository owner (user or organization)
	repoName := action.Repo // Repository name without owner prefix potentially with subpath
	ref := action.Ref       // Current reference (tag, branch, or SHA)

	// Extract repository name for API calls
	// For actions with subpaths like "owner/repo/subpath", we just need "repo" for the API
	repoParts := strings.SplitN(repoName, "/", 2) //nolint:mnd
	repoNameForAPI := repoParts[0]                // Just the repository name without subpath

	// Validate that we were able to extract a repository name
	if repoNameForAPI == "" {
		log.Printf(
			"❌ Could not extract repository name from '%s' for action on line %d. Skipping.",
			repoName,
			lineNum,
		)
		return nil // Continue processing other references
	}

	// Construct the full path for the 'uses' string (owner/repo/subpath)
	// This is the complete reference as it appears in the workflow file
	fullPathForUses := fmt.Sprintf("%s/%s", owner, repoName)

	// Check if we're in update mode (updating existing SHAs to latest)
	if Update {
		// Update mode: Find the latest version of the action
		log.Printf("🔍  Finding latest version for action: %s (repo: %s/%s) (line %d)",
			fullPathForUses, owner, repoNameForAPI, lineNum)

		// Get the latest reference and its commit SHA
		latestRef, commitSHA, err := githubclient.GetLatestActionRef(
			ctx,
			client,
			owner,
			repoNameForAPI,
		)
		if err != nil || commitSHA == "" || latestRef == "" {
			// Log an error if we can't find the latest version
			log.Printf(
				"❌  Error finding latest ref/SHA for action %s/%s: %v. Skipping update for line %d.",
				owner,
				repoNameForAPI,
				err,
				lineNum,
			)
			return nil // Continue processing other actions
		}

		// Create the new action reference string with SHA + comment
		newUsesValue := fmt.Sprintf(
			"%s@%s #%s", // Format: owner/repo/subpath@sha #ref
			fullPathForUses,
			commitSHA, // Use the full SHA for pinning
			latestRef, // Include latest reference as a comment
		)

		// Log the update details
		log.Printf(
			"  Updating %s@%s to SHA %s (latest ref: %s)",
			fullPathForUses,
			ref,
			commitSHA[:8], // Show only first 8 chars of SHA for readability
			latestRef,
		)

		// Check if the action is already up-to-date
		if isSHA && ref == commitSHA {
			// If current reference is already the latest SHA, no update needed
			log.Printf(
				"  Action %s already up-to-date with SHA %s (latest ref: %s). No change needed.",
				fullPathForUses,
				commitSHA[:8],
				latestRef,
			)
		} else {
			// Store the update in the map and increment counter
			updates[lineNum] = newUsesValue
			*updatesMade++
		}

		return nil // Successfully processed action in update mode
	} else {
		// Pin mode: Pin existing references to their current SHA

		// If the reference is already a SHA, no need to pin it
		if isSHA {
			log.Printf("ℹ️  Action '%s' on line %d already pinned to SHA: %s", usesValue, lineNum, ref)
			return nil // Already pinned, no update needed
		}

		// Resolve the current reference to its commit SHA
		log.Printf("🔍  Resolving SHA for action: %s (repo: %s/%s) @%s (line %d)",
			fullPathForUses, owner, repoNameForAPI, ref, lineNum)
		commitSHA, err := githubclient.ResolveRefToSHA(ctx, client, owner, repoNameForAPI, ref)
		if err != nil || commitSHA == "" {
			// Log an error if we can't resolve the SHA
			log.Printf("❌  Error resolving ref '%s' to SHA for action %s/%s: %v. Skipping update for line %d.",
				ref, owner, repoNameForAPI, err, lineNum)
			return nil // Continue processing other actions
		}

		// Create the new action reference string with SHA + comment
		newUsesValue := fmt.Sprintf("%s@%s #%s", fullPathForUses, commitSHA, ref)
		log.Printf("  Pinned action %s@%s to SHA %s", fullPathForUses, ref, commitSHA[:8])

		// Store the update in the map and increment counter
		updates[lineNum] = newUsesValue
		*updatesMade++

		return nil // Successfully processed action in pin mode
	}
}

// resolveWorkflowRef determines the appropriate Git reference to use for a reusable workflow.
// If no reference is provided, it fetches the repository's default branch.
//
// - ctx: The context for API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - owner: The owner (user or organization) of the GitHub repository.
// - repoNameForAPI: The repository name to use in API calls (without subpaths).
// - currentRef: The current reference specified in the workflow, may be empty.
// - fullPathForUses: The complete "uses" path for logging purposes.
//
// Returns:
//   - string: The resolved branch name (either the provided ref or default branch)
//   - string: The reference to use in comments (for tracking original reference)
//   - error: An error if default branch resolution fails when needed
func resolveWorkflowRef(
	ctx context.Context,
	client *github.Client,
	owner, repoNameForAPI, currentRef, fullPathForUses string,
) (string, string, error) {
	branchName := currentRef
	originalRefForComment := currentRef

	// Check if a reference was provided in the workflow file
	if branchName == "" {
		// No reference specified, so we need to get the default branch
		log.Printf(
			"ℹ️ No ref specified for workflow %s. Resolving default branch for %s/%s.",
			fullPathForUses,
			owner,
			repoNameForAPI,
		)

		// Make an API call to get repository information
		// This will include the default branch name
		repoInfo, _, err := client.Repositories.Get(ctx, owner, repoNameForAPI)
		if err != nil {
			return "", "", fmt.Errorf(
				"error getting repository info for %s/%s to find default branch: %w",
				owner,
				repoNameForAPI,
				err,
			)
		}

		// Verify that the default branch information is available
		// DefaultBranch is a pointer and could be nil, or could point to an empty string
		if repoInfo.DefaultBranch == nil || *repoInfo.DefaultBranch == "" {
			return "", "", fmt.Errorf(
				"could not determine default branch for %s/%s",
				owner,
				repoNameForAPI,
			)
		}

		// Use the default branch as the reference
		branchName = *repoInfo.DefaultBranch

		// Store the default branch name as the original reference for commenting purposes
		// This helps track that we automatically resolved to the default branch
		originalRefForComment = branchName

		log.Printf("  Using default branch '%s' for %s/%s",
			branchName,
			owner,
			repoNameForAPI,
		)
	}

	// Return both the branch to use and the original reference (for comments)
	return branchName, originalRefForComment, nil
}

// ApplyUpdatesToLines takes the original content of a file and a map of line numbers
// to new string values, and reconstructs the content with the specified lines replaced.
// It preserves original line endings and indentation where possible for 'uses:' lines.
//
// - originalContent: The string content of the file before modification.
// - updates: A map where keys are 1-based line numbers and values are the replacement strings.
//
// Returns: The modified content as a string, and an error if processing fails
func applyUpdatesToLines(originalContent string, updates map[int]string) (string, error) {
	// Split the original content into individual lines. strings.Split handles various line endings.
	lines := strings.Split(originalContent, "\n")
	var output strings.Builder
	// Pre-allocate capacity for the output string builder to improve performance,
	// estimating the potential increase in size due to added comments.
	output.Grow(
		len(originalContent) + len(updates)*20,
	) // Rough estimate: 20 characters per update comment.

	// Iterate through the lines, using a 0-based index `i`.
	for i, line := range lines {
		// Calculate the 1-based line number for lookup in the 'updates' map.
		lineNumber := i + 1
		// Check if there is an update specified for the current line number.
		if newUsesValue, ok := updates[lineNumber]; ok {
			// An update exists for this line.
			// Trim whitespace from the beginning of the line to check if it starts with 'uses:'.
			trimmedLine := strings.TrimSpace(line)
			// Verify that the line actually starts with 'uses:' (case-sensitive as per YAML spec).
			if strings.HasPrefix(trimmedLine, "uses:") ||
				strings.HasPrefix(trimmedLine, "- uses:") {
				// Identify the leading indentation (spaces and tabs) of the original line.
				indentation := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				// Construct the new line, preserving the dash if it exists
				newLine := ""
				if strings.HasPrefix(trimmedLine, "- uses:") {
					// If it has the dash prefix, maintain it in the updated line
					newLine = indentation + "- uses: " + newUsesValue
				} else {
					// Regular "uses:" line without dash
					newLine = indentation + "uses: " + newUsesValue
				}
				// Write the new line to the output buffer.
				output.WriteString(newLine)
			} else {
				// If an update was mapped to this line number, but the line content doesn't look like
				// a 'uses:' entry, log a warning. This indicates a potential issue with the line
				// number reported by the YAML parser for the 'uses' node. In this case, we append
				// the original line to avoid corrupting the file.
				log.Printf("Warning: Update found for line %d, but line content '%s' does not look like a 'uses:' line. Appending original.", lineNumber, line)
				output.WriteString(line)
			}
		} else {
			// No update for this line, append the original line content.
			output.WriteString(line)
		}

		// Add the newline character back. This is added after each line except potentially the very last one.
		// strings.Split(content, "\n") will produce a final empty string if the original content ended with a newline.
		// We want to preserve the original ending: if the original content ended with a newline, the split results
		// in `len(lines)` items, and the last item is empty. If it didn't end with a newline, `len(lines)` is
		// the number of visual lines, and the last item contains content.
		// The condition `i < len(lines)-1` correctly adds a newline after every line *except* the last one produced by the split.
		if i < len(lines)-1 {
			output.WriteString("\n")
		}
	}

	// Return the accumulated content from the string builder.
	return output.String(), nil
}

// UpdateWorkflowActionSHAs reads a workflow file, parses its YAML structure,
// identifies GitHub Actions needing SHA pinning, resolves the SHAs, and
// modifies the file content in memory before writing it back.
//
// - ctx: The context for API calls, allows for cancellation/timeouts.
// - client: The initialized GitHub client for making API requests.
// - filePath: The path to the workflow file to process.
//
// Returns:
//   - int: The number of actions updated in the file
//   - error: An error if reading, parsing, resolving, or writing fails
func UpdateWorkflowActionSHAs(
	ctx context.Context,
	client *github.Client,
	filePath string,
) (int, error) {
	// Validate the workflow file path to prevent security issues
	// This ensures the path doesn't contain dangerous patterns like path traversal
	if err := utils.ValidateWorkflowFilePath(filePath); err != nil {
		return 0, err // Return the validation error without modification
	}

	// Read the file content into memory
	// The nolint:gosec comment suppresses a security scanner warning about using
	// a variable filepath - we've already validated it above
	data, err := os.ReadFile(filePath) //nolint:gosec
	if err != nil {
		return 0, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	// Skip processing if the file is empty
	if len(data) == 0 {
		log.Printf("Skipping empty file: %s", filePath)
		return 0, nil // Return 0 updates and no error
	}

	// Parse the workflow YAML into a structured AST (Abstract Syntax Tree)
	// This preserves line numbers and structure for precise updates
	root, err := parser.ParseWorkflowYAML(filePath, data)
	if err != nil {
		return 0, err // Return any parsing errors
	}

	// If the parser returned nil (e.g., for an empty document), skip processing
	if root == nil {
		return 0, nil // Return 0 updates and no error
	}

	// Initialize a map to store the identified updates
	// Keys are line numbers, values are the new 'uses:' strings
	updates := make(map[int]string)
	updatesMade := 0 // Counter for updates identified

	// Recursively traverse the YAML AST to find 'uses:' keys and populate the updates map
	// We start from the first content node of the root (usually a DocumentNode or MappingNode)
	if len(root.Content) > 0 {
		err = findUpdatesInNodes(ctx, client, root.Content[0], updates, &updatesMade)
		if err != nil {
			// Return the number of updates found before the error and the error itself
			return updatesMade, err
		}
	}

	// Apply updates if any were identified
	if updatesMade > 0 {
		log.Printf("Applying %d update(s) to %s", updatesMade, filePath)

		// Modify the original file content line by line with the updates
		updatedContent, err := applyUpdatesToLines(string(data), updates)
		if err != nil {
			return updatesMade, fmt.Errorf(
				"error applying updates to lines for %s: %w",
				filePath,
				err,
			)
		}

		// Write the modified content back to the original file
		// The nolint comments suppress security scanner warnings:
		// - gosec: for using a variable filepath (already validated)
		// - mnd: for using a "magic number" for file permissions
		err = os.WriteFile( //nolint:gosec //nolint:mnd
			filePath,
			[]byte(updatedContent),
			0o640, //nolint:mnd
		)
		if err != nil {
			return updatesMade, fmt.Errorf("error writing updated file %s: %w", filePath, err)
		}
	}

	// Return the total number of updates made and nil error if successful
	return updatesMade, nil
}
