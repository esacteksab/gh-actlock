// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var force bool // Flag variable

func init() {
	rootCmd.AddCommand(clearCmd)
	// Define the --force / -f flag
	clearCmd.Flags().
		BoolVarP(&force, "force", "f", false, "force deletion without confirmation")
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear local application cache",
	Long: `Deletes the gh-actlock cache directory located within the user's
standard cache location (e.g., $XDG_CACHE_HOME/gh-actlock on Linux).
Requires the --force flag to proceed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the user's cache directory
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("failed to get user cache directory: %w", err)
		}

		// Define the application's cache directory name
		appCacheDirName := "gh-actlock"
		cachePath := filepath.Join(userCacheDir, appCacheDirName)

		// 1. Check if the target path exists and is accessible
		_, err = os.Stat(cachePath)
		if err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist - this is a successful 'clear' state.
				fmt.Printf(
					"Cache directory '%s' does not exist. Nothing to clear.\n",
					cachePath,
				)
				return nil // Indicate success, as the directory is gone.
			}
			// Other error trying to stat the directory (e.g., permissions)
			return fmt.Errorf("failed to check status of cache directory '%s': %w", cachePath, err)
		}

		// 2. If it exists, check for the force flag
		if !force {
			// Require the force flag if the directory exists
			return fmt.Errorf(
				"cache directory '%s' exists. Use the -f or --force flag to confirm deletion",
				cachePath,
			)
		}

		// 3. Force flag provided, proceed with deletion
		fmt.Printf("Removing cache directory '%s'...\n", cachePath) // Inform user
		err = os.RemoveAll(cachePath)
		if err != nil {
			// Error during the actual removal
			return fmt.Errorf("failed removing cache directory '%s': %w", cachePath, err)
		}

		// 4. Verify deletion
		_, err = os.Stat(cachePath)
		if os.IsNotExist(err) {
			// Expected outcome: stat fails with IsNotExist
			fmt.Printf("Cache directory '%s' removed successfully.\n", cachePath)
		} else if err != nil {
			// Unexpected error trying to verify (e.g., permissions changed mid-operation?)
			// The directory might be gone, but we couldn't confirm.
			return fmt.Errorf("removed '%s', but failed to verify removal status: %w", cachePath, err)
		} else {
			// Stat succeeded *after* RemoveAll - this means deletion failed silently? Very unlikely but possible.
			return fmt.Errorf("attempted to remove cache directory '%s', but it still exists", cachePath)
		}

		return nil
	},
}
