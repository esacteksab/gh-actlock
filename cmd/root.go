// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v71/github"
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
		fmt.Fprintln(os.Stderr, err)
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

		// Check the current GitHub API rate limit. This is helpful for debugging potential rate limit issues.
		githubclient.CheckRateLimit(ctx, client)

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

// --- Workflow Handler ---

// handleWorkflowReference handles pinning or updating reusable workflow references.
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
	owner := action.Name
	repoField := action.Repo
	ref := action.Ref

	// Extract repository name for API calls
	repoParts := strings.SplitN(repoField, "/", 2) //nolint:mnd
	repoNameForAPI := repoParts[0]
	if repoNameForAPI == "" {
		log.Printf(
			"❌ Could not extract repository name from '%s' for workflow on line %d. Skipping.",
			repoField,
			lineNum,
		)
		return nil // Continue processing
	}

	// Full path for the 'uses' string (owner/repo/path)
	fullPathForUses := fmt.Sprintf("%s/%s", owner, repoField)

	// --- Workflow Update Mode ---
	if Update {
		log.Printf("🔍 Finding latest version for workflow: %s (repo: %s/%s) (line %d)",
			fullPathForUses, owner, repoNameForAPI, lineNum)

		latestRef, commitSHA, err := githubclient.GetLatestActionRef(
			ctx,
			client,
			owner,
			repoNameForAPI,
		)
		if err != nil || commitSHA == "" || latestRef == "" {
			log.Printf(
				"❌ Error finding latest ref/SHA for workflow repo %s/%s: %v. Skipping update for line %d.",
				owner,
				repoNameForAPI,
				err,
				lineNum,
			)
			return nil // Continue processing
		}

		newUsesValue := fmt.Sprintf("%s@%s # updated to %s", fullPathForUses, commitSHA, latestRef)
		log.Printf(
			"  Updating workflow %s to SHA %s (latest ref: %s)",
			fullPathForUses,
			commitSHA[:8],
			latestRef,
		)

		if isSHA && ref == commitSHA {
			log.Printf(
				"  Workflow %s already up-to-date with SHA %s (latest ref: %s). No change needed.",
				fullPathForUses,
				commitSHA[:8],
				latestRef,
			)
		} else {
			updates[lineNum] = newUsesValue
			*updatesMade++
		}
		return nil // Processed workflow in update mode

		// --- Workflow Pinning Mode ---
	} else {
		if isSHA {
			log.Printf("ℹ️  Workflow '%s' on line %d already pinned to SHA: %s", usesValue, lineNum, ref)
			return nil // Already pinned
		}

		branchName, originalRefForComment, err := resolveWorkflowRef(ctx, client, owner, repoNameForAPI, ref, fullPathForUses)
		if err != nil {
			log.Printf("❌  Skipping pin for workflow '%s' on line %d: %v", usesValue, lineNum, err)
			return nil // Continue processing
		}

		log.Printf("🔍  Pinning workflow: %s@%s (line %d) (repo: %s)", fullPathForUses, branchName, lineNum, repoNameForAPI)

		commitSHA, err := githubclient.ResolveRefToSHA(ctx, client, owner, repoNameForAPI, branchName)
		if err != nil || commitSHA == "" {
			log.Printf("❌  Error resolving ref '%s' to SHA for workflow %s/%s: %v. Skipping update for line %d.",
				branchName, owner, repoNameForAPI, err, lineNum)
			return nil // Continue processing
		}

		newUsesValue := fmt.Sprintf("%s@%s # pinned from %s", fullPathForUses, commitSHA, originalRefForComment)
		log.Printf("  Pinned workflow %s@%s to SHA %s", fullPathForUses, originalRefForComment, commitSHA[:8])

		updates[lineNum] = newUsesValue
		*updatesMade++
		return nil // Processed workflow in pinning mode
	}
}

// --- Action Handler ---

// handleActionReference handles pinning or updating regular action references.
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
	owner := action.Name
	repoName := action.Repo // For actions, Repo field is just the repo name
	ref := action.Ref

	// --- Action Update Mode ---
	if Update {
		log.Printf("🔍  Finding latest version for action: %s/%s (currently @%s) (line %d)",
			owner, repoName, ref, lineNum)

		latestRef, commitSHA, err := githubclient.GetLatestActionRef(ctx, client, owner, repoName)
		if err != nil || commitSHA == "" || latestRef == "" {
			log.Printf(
				"❌  Error finding latest ref/SHA for action %s/%s: %v. Skipping update for line %d.",
				owner,
				repoName,
				err,
				lineNum,
			)
			return nil // Continue processing
		}

		newUsesValue := fmt.Sprintf(
			"%s/%s@%s # updated to %s",
			owner,
			repoName,
			commitSHA,
			latestRef,
		)
		log.Printf(
			"  Updating %s/%s@%s to SHA %s (latest ref: %s)",
			owner,
			repoName,
			ref,
			commitSHA[:8],
			latestRef,
		)

		if isSHA && ref == commitSHA {
			log.Printf(
				"  Action %s/%s already up-to-date with SHA %s (latest ref: %s). No change needed.",
				owner,
				repoName,
				commitSHA[:8],
				latestRef,
			)
		} else {
			updates[lineNum] = newUsesValue
			*updatesMade++
		}
		return nil // Processed action in update mode

		// --- Action Pinning Mode ---
	} else {
		if isSHA {
			log.Printf("ℹ️  Action '%s' on line %d already pinned to SHA: %s", usesValue, lineNum, ref)
			return nil // Already pinned
		}

		log.Printf("🔍  Resolving SHA for action: %s/%s@%s (line %d)", owner, repoName, ref, lineNum)

		commitSHA, err := githubclient.ResolveRefToSHA(ctx, client, owner, repoName, ref)
		if err != nil || commitSHA == "" {
			log.Printf("❌  Error resolving ref '%s' to SHA for action %s/%s: %v. Skipping update for line %d.",
				ref, owner, repoName, err, lineNum)
			return nil // Continue processing
		}

		newUsesValue := fmt.Sprintf("%s/%s@%s # pinned from %s", owner, repoName, commitSHA, ref)
		log.Printf("  Pinned action %s/%s@%s to SHA %s", owner, repoName, ref, commitSHA[:8])

		updates[lineNum] = newUsesValue
		*updatesMade++
		return nil // Processed action in pinning mode
	}
}

// --- Helper Function ---

// resolveWorkflowRef determines the correct ref (branch/tag) to use for pinning a workflow,
// handling the case where no ref is specified (resolving the default branch).
// Returns the ref to resolve (branchName), the ref to use in comments (originalRef), and any error.
func resolveWorkflowRef(
	ctx context.Context,
	client *github.Client,
	owner, repoNameForAPI, currentRef, fullPathForUses string,
) (string, string, error) {
	branchName := currentRef
	originalRefForComment := currentRef

	if branchName == "" {
		log.Printf(
			"ℹ️ No ref specified for workflow %s. Resolving default branch for %s/%s.",
			fullPathForUses,
			owner,
			repoNameForAPI,
		)
		repoInfo, _, err := client.Repositories.Get(ctx, owner, repoNameForAPI)
		if err != nil {
			return "", "", fmt.Errorf(
				"error getting repository info for %s/%s to find default branch: %w",
				owner,
				repoNameForAPI,
				err,
			)
		}
		if repoInfo.DefaultBranch == nil || *repoInfo.DefaultBranch == "" {
			return "", "", fmt.Errorf(
				"could not determine default branch for %s/%s",
				owner,
				repoNameForAPI,
			)
		}
		branchName = *repoInfo.DefaultBranch
		originalRefForComment = branchName // Use resolved default branch in comment
		log.Printf("  Using default branch '%s' for %s/%s", branchName, owner, repoNameForAPI)
	}
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

func UpdateWorkflowActionSHAs(
	ctx context.Context,
	client *github.Client,
	filePath string,
) (int, error) {
	// Validate the workflow file path
	if err := utils.ValidateWorkflowFilePath(filePath); err != nil {
		return 0, err
	}

	// Read the file content
	data, err := os.ReadFile(filePath) //nolint:gosec
	if err != nil {
		return 0, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	if len(data) == 0 {
		log.Printf("Skipping empty file: %s", filePath)
		return 0, nil
	}

	// Parse YAML
	root, err := parser.ParseWorkflowYAML(filePath, data)
	if err != nil {
		return 0, err
	}

	if root == nil {
		return 0, nil
	}

	// Find updates
	updates := make(map[int]string)
	updatesMade := 0

	if len(root.Content) > 0 {
		err = findUpdatesInNodes(ctx, client, root.Content[0], updates, &updatesMade)
		if err != nil {
			return updatesMade, err
		}
	}

	// Apply updates
	if updatesMade > 0 {
		log.Printf("Applying %d update(s) to %s", updatesMade, filePath)

		updatedContent, err := applyUpdatesToLines(string(data), updates)
		if err != nil {
			return updatesMade, fmt.Errorf(
				"error applying updates to lines for %s: %w",
				filePath,
				err,
			)
		}

		err = os.WriteFile( //nolint:gosec
			filePath,
			[]byte(updatedContent),
			0o640, //nolint:mnd
		)
		if err != nil {
			return updatesMade, fmt.Errorf("error writing updated file %s: %w", filePath, err)
		}
	}

	return updatesMade, nil
}
