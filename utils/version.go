// SPDX-License-Identifier: MIT

package utils

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// BuildVersion constructs a formatted version string using build information.
// It combines the application version with details about the build environment
// and compilation settings for diagnostic and informational purposes.
//
// -Version: The semantic version of the application (e.g., "v1.0.0").
// -Commit: The Git commit hash of the source code used for the build.
// -Date: The timestamp when the build was created.
// -BuiltBy: The entity (person, CI system) that created the build.
// Returns: A multi-line string containing formatted version and build information.
func BuildVersion(Version, Commit, Date, BuiltBy string) string {
	// Start with the basic version number
	result := Version

	// Add Git commit hash if available
	if Commit != "" {
		result = fmt.Sprintf("%s\nCommit: %s\n", result, Commit)
	}

	// Add build timestamp if available
	if Date != "" {
		result = fmt.Sprintf("%sBuilt at: %s\n", result, Date)
	}

	// Add builder information if available
	if BuiltBy != "" {
		result = fmt.Sprintf("%sBuilt by: %s\n", result, BuiltBy)
	}

	// Add Go runtime information (operating system and architecture)
	// GOOS and GOARCH are runtime constants that identify the target platform
	result = fmt.Sprintf(
		"%sGOOS: %s\nGOARCH: %s\n", result, runtime.GOOS, runtime.GOARCH,
	)

	// Attempt to retrieve Go module information from the binary
	// This provides details about the main module's version and checksum
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
		result = fmt.Sprintf(
			"%smodule Version: %s, checksum: %s",
			result,
			info.Main.Version, // The version of the main module (e.g., (devel) for local builds)
			info.Main.Sum,     // The checksum of the module as recorded in go.sum
		)
	}

	return result
}
