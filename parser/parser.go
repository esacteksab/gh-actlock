// SPDX-License-Identifier: MIT

package parser

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/esacteksab/gh-actlock/githubclient"
)

// WorkflowAction represents an action reference (uses: xxx/yyy@version)
// This struct holds the parsed components of a GitHub Action reference.
type WorkflowAction struct {
	Name string // Owner or organization name
	Repo string // Repository name (potentially including subpath)
	Ref  string // Tag, branch, or SHA reference
	Type string // Action type: "github", "docker", "local", or "unknown"
}

// Workflow represents the GitHub Actions workflow file structure
// This matches the YAML structure of GitHub Actions workflow files.
type Workflow struct {
	Name        string         `yaml:"name,omitempty"`        // Name of the workflow
	RunName     string         `yaml:"run-name,omitempty"`    // Dynamic name for workflow runs
	On          any            `yaml:"on"`                    // Event triggers for the workflow
	Permissions any            `yaml:"permissions,omitempty"` // Workflow-level permissions
	Env         map[string]any `yaml:"env,omitempty"`         // Workflow-level environment variables
	Defaults    *Defaults      `yaml:"defaults,omitempty"`    // Default settings for all jobs
	Concurrency any            `yaml:"concurrency,omitempty"` // Concurrency group settings
	Jobs        map[string]Job `yaml:"jobs"`                  // The jobs that make up the workflow
}

// Defaults represents default settings for all jobs
type Defaults struct {
	Run *RunDefaults `yaml:"run,omitempty"` // Default run settings
}

// RunDefaults represents default run settings
type RunDefaults struct {
	Shell            string `yaml:"shell,omitempty"`             // Default shell to use
	WorkingDirectory string `yaml:"working-directory,omitempty"` // Default working directory
}

// Job represents a job within a workflow
type Job struct {
	Name            string               `yaml:"name,omitempty"`              // Display name of the job
	Needs           any                  `yaml:"needs,omitempty"`             // Dependencies on other jobs
	Permissions     any                  `yaml:"permissions,omitempty"`       // Job-level permissions
	RunsOn          any                  `yaml:"runs-on,omitempty"`           // Runner type(s) to use
	Environment     any                  `yaml:"environment,omitempty"`       // Deployment environment
	Outputs         map[string]string    `yaml:"outputs,omitempty"`           // Job outputs for other jobs
	Env             map[string]any       `yaml:"env,omitempty"`               // Job-level environment variables
	Defaults        *Defaults            `yaml:"defaults,omitempty"`          // Job-specific default settings
	If              any                  `yaml:"if,omitempty"`                // Conditional execution
	Steps           []Step               `yaml:"steps,omitempty"`             // Steps to execute in the job
	TimeoutMinutes  any                  `yaml:"timeout-minutes,omitempty"`   // Job timeout
	Strategy        *Strategy            `yaml:"strategy,omitempty"`          // Build matrix strategy
	ContinueOnError any                  `yaml:"continue-on-error,omitempty"` // Whether to continue on failure
	Container       any                  `yaml:"container,omitempty"`         // Container to run the job in
	Services        map[string]Container `yaml:"services,omitempty"`          // Service containers
	Concurrency     any                  `yaml:"concurrency,omitempty"`       // Job-level concurrency
	Uses            string               `yaml:"uses,omitempty"`              // Reusable workflow reference
	With            map[string]any       `yaml:"with,omitempty"`              // Inputs for reusable workflow
	Secrets         any                  `yaml:"secrets,omitempty"`           // Secrets for reusable workflow
}

// Step represents a step within a job
type Step struct {
	ID               string         `yaml:"id,omitempty"`                // Step identifier
	If               any            `yaml:"if,omitempty"`                // Conditional execution
	Name             string         `yaml:"name,omitempty"`              // Display name of the step
	Uses             string         `yaml:"uses,omitempty"`              // Action reference
	Run              string         `yaml:"run,omitempty"`               // Command to run
	WorkingDirectory string         `yaml:"working-directory,omitempty"` // Step-specific working directory
	Shell            string         `yaml:"shell,omitempty"`             // Step-specific shell
	With             map[string]any `yaml:"with,omitempty"`              // Inputs for the action
	Env              map[string]any `yaml:"env,omitempty"`               // Step-level environment variables
	ContinueOnError  any            `yaml:"continue-on-error,omitempty"` // Whether to continue on failure
	TimeoutMinutes   any            `yaml:"timeout-minutes,omitempty"`   // Step timeout
}

// Strategy represents a build matrix strategy
type Strategy struct {
	Matrix      any `yaml:"matrix"`                 // Matrix configuration
	FailFast    any `yaml:"fail-fast,omitempty"`    // Whether to cancel all jobs if any fail
	MaxParallel any `yaml:"max-parallel,omitempty"` // Maximum parallel jobs
}

// Container represents a container configuration
type Container struct {
	Image       string                `yaml:"image"`                 // Container image to use
	Credentials *ContainerCredentials `yaml:"credentials,omitempty"` // Registry credentials
	Env         map[string]any        `yaml:"env,omitempty"`         // Container environment variables
	Ports       []any                 `yaml:"ports,omitempty"`       // Ports to expose
	Volumes     []string              `yaml:"volumes,omitempty"`     // Volumes to mount
	Options     string                `yaml:"options,omitempty"`     // Additional Docker options
}

// ContainerCredentials represents credentials for a container
type ContainerCredentials struct {
	Username string `yaml:"username"` // Registry username
	Password string `yaml:"password"` // Registry password
}

// Environment represents an environment configuration
type Environment struct {
	Name string `yaml:"name"`          // Environment name
	URL  string `yaml:"url,omitempty"` // Environment URL
}

// Concurrency represents concurrency settings
type Concurrency struct {
	Group            string `yaml:"group"`                        // Concurrency group name
	CancelInProgress any    `yaml:"cancel-in-progress,omitempty"` // Whether to cancel in-progress runs
}

// ParseActionReference parses a "uses:" line into owner, repo, and ref
//
// - uses: The raw action reference string (e.g., "actions/checkout@v4")
//
// Returns: A WorkflowAction struct with the parsed components and an error if parsing fails
func ParseActionReference(uses string) (WorkflowAction, error) {
	action := WorkflowAction{Type: "unknown"} // Initialize with default type
	if uses == "" {
		return action, errors.New("empty action reference")
	}

	// Check for local path action (starts with ./ or ../)
	// Local actions reference code in the same repository
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") {
		action.Type = "local"
		action.Repo = uses // Store the local path in Repo field for simplicity
		return action, nil
	}

	// Check if it's a Docker action (starts with docker://)
	// Docker actions run commands in a Docker container
	if strings.HasPrefix(uses, "docker://") {
		action.Type = "docker"
		fullImage := uses[len("docker://"):] // Remove the "docker://" prefix

		// Split the Docker image reference into name and tag parts
		parts := strings.SplitN(fullImage, ":", 2) //nolint:mnd // Split at first colon
		action.Repo = parts[0]                     // Store Docker image name in Repo field

		if len(parts) > 1 {
			action.Ref = parts[1] // Store Docker tag in Ref field
		} else {
			action.Ref = "latest" // Default Docker tag if none specified
		}
		return action, nil
	}

	// If we're here, assume it's a standard GitHub action format: owner/repo[/path]@ref
	action.Type = "github" // Set type to GitHub action

	// Split at the @ symbol to separate repository path from reference
	parts := strings.SplitN(uses, "@", 2) //nolint:mnd // Split at first @ symbol
	repoPath := parts[0]                  // The repository path part (owner/repo[/path])

	if len(parts) > 1 {
		action.Ref = parts[1] // The reference part (tag, branch, or SHA)
	} else {
		// GitHub actions require an explicit reference for reliable pinning
		// Without a reference, we can't determine which version to pin to
		return action, fmt.Errorf("github action reference '%s' missing explicit @ref (tag/branch/sha)", uses)
	}

	// Split repository path into owner/repo parts
	pathParts := strings.SplitN(repoPath, "/", 2) //nolint:mnd // Split at first slash
	if len(pathParts) == 2 {                      //nolint:mnd
		action.Name = pathParts[0] // Owner or organization name
		action.Repo = pathParts[1] // Repository name (may include subpath)
	} else {
		// Standard format requires both owner and repo
		return action, fmt.Errorf("invalid GitHub action format '%s', expected 'owner/repo@ref'", uses)
	}

	// Basic validation to ensure all required parts are present
	if action.Name == "" || action.Repo == "" || action.Ref == "" {
		return action, fmt.Errorf("incomplete GitHub action reference '%s'", uses)
	}

	return action, nil
}

// FindAllActions finds all action references in a workflow struct
//
// - workflow: The parsed workflow structure to analyze
//
// Returns: A slice of WorkflowAction objects representing all GitHub actions used in the workflow
func FindAllActions(workflow *Workflow) []WorkflowAction {
	var actions []WorkflowAction
	if workflow == nil {
		return actions // Return empty slice if workflow is nil
	}

	// Iterate through all jobs in the workflow
	for _, job := range workflow.Jobs {
		// Handle job-level "uses" (for reusable workflows)
		if job.Uses != "" {
			action, err := ParseActionReference(job.Uses)
			// Only add valid GitHub actions that might need pinning
			// Skip local actions and Docker containers
			if err == nil && action.Type == "github" {
				actions = append(actions, action)
			} else if err != nil {
				fmt.Printf("Warning: Skipping job 'uses: %s': %v\n", job.Uses, err)
			}
		}

		// Handle step-level "uses" (for actions)
		for _, step := range job.Steps {
			if step.Uses != "" {
				action, err := ParseActionReference(step.Uses)
				// Only add valid GitHub actions that might need pinning
				// Skip local actions and Docker containers
				if err == nil && action.Type == "github" {
					actions = append(actions, action)
				} else if err != nil {
					fmt.Printf("Warning: Skipping step 'uses: %s': %v\n", step.Uses, err)
				}
			}
		}
	}

	return actions
}

// GetRefType determines the type of a Git reference string.
//
// - ref: The Git reference string to analyze
//
// Returns: A string indicating the reference type: "sha", "short_sha", "version", "branch", or "unknown"
func GetRefType(ref string) string {
	// If reference is empty, return "unknown"
	if ref == "" {
		return "unknown"
	}

	refLength := len(ref)

	// --- Check for specific formats in order of specificity ---

	// 1. Check if it's a full SHA (40 hexadecimal characters)
	if refLength == githubclient.SHALength {
		if githubclient.IsHexString(ref) {
			return "sha" // It's a full-length SHA hash
		}
		// If it's 40 characters but not hex, fall through to other checks
	}

	// 2. Check for short SHA (7-39 hexadecimal characters)
	const minShortSHALength = 7 // GitHub often uses 7 characters as minimum for short SHAs
	if refLength >= minShortSHALength && refLength < githubclient.SHALength {
		if githubclient.IsHexString(ref) {
			return "short_sha" // It's a shortened SHA hash
		}
		// If it's in the right length range but not hex, fall through
	}

	// 3. Check if it's a simple alphanumeric reference
	if IsSimpleRef(ref) { // Only contains letters and digits
		// Special case for version tags that start with "v" followed by a number
		if strings.HasPrefix(ref, "v") && refLength > 1 {
			// Check if the character after 'v' is a digit (0-9)
			numberPart := ref[1:]
			// Only check the first character after 'v'
			if len(numberPart) > 0 && numberPart[0] >= '0' && numberPart[0] <= '9' {
				return "version" // It's a version tag like "v1", "v2.0", etc.
			}
		}
		// If it's alphanumeric but not a version tag
		return "branch" // Assume it's a branch name like "main", "master"
	}

	// If none of the above patterns match
	return "unknown" // Could be something like "refs/tags/v1.0" or other complex reference
}

// IsSimpleRef checks if a reference string contains only letters and digits.
//
// - ref: The reference string to check
//
// Returns: true if the string contains only letters (a-z, A-Z) and digits (0-9), false otherwise
func IsSimpleRef(ref string) bool {
	if ref == "" {
		return false // Empty string is not a simple reference
	}

	// Check each character in the string
	for _, r := range ref {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'

		// If character is neither a letter nor a digit
		if !isLetter && !isDigit {
			return false // String contains non-alphanumeric character
		}
	}

	// All characters passed the check
	return true // String contains only letters and digits
}

// ParseWorkflowYAML parses YAML content from a workflow file into a structured Node tree.
// It preserves line numbers and structure for precise updates and handles empty files gracefully.
//
// - filePath: The path to the workflow file being parsed (used for error messages).
// - data: The raw YAML content as a byte slice to be parsed.
//
// Returns:
//   - A pointer to the root yaml.Node of the parsed YAML structure.
//   - nil and no error if the file is empty.
//   - nil and an error if parsing fails.
func ParseWorkflowYAML(filePath string, data []byte) (*yaml.Node, error) {
	// Check if the file is empty before attempting to parse.
	// Empty files are valid (not an error) but require special handling.
	if len(data) == 0 {
		log.Printf("Skipping empty file: %s", filePath)
		return nil, nil
	}

	// Unmarshal the YAML content into a yaml.Node structure.
	// This creates an Abstract Syntax Tree (AST) that preserves line numbers
	// and structural information needed for precise updates.
	var root yaml.Node
	err := yaml.Unmarshal(data, &root)
	if err != nil {
		// Double-check if the error is specifically related to an empty file.
		// This provides redundancy with the initial check for empty data.
		if strings.Contains(err.Error(), "EOF") && len(data) == 0 {
			log.Printf("Skipping empty file: %s", filePath)
			return nil, nil
		}

		// For any other parsing error, return a descriptive error with the file path.
		return nil, fmt.Errorf("error parsing YAML file %s: %w", filePath, err)
	}

	// Return the parsed YAML structure on success
	return &root, nil
}

// IsReusableWorkflow checks if an action reference is actually a reusable workflow
func IsReusableWorkflow(ref string) bool {
	return strings.Contains(ref, "/.github/workflows/") ||
		strings.Contains(ref, ".github/workflows/")
}
