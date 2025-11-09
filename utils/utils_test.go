// SPDX-License-Identifier: MIT

package utils

import (
	"os"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ValidateFilePath(t *testing.T) {
	CreateLogger(true)
	t.Setenv("ACTLOCK_DEBUG", "true")
	if Logger == nil {
		Logger = log.NewWithOptions(os.Stderr, log.Options{Level: log.InfoLevel})
	}

	type args struct {
		path string
	}
	tests := []struct {
		name       string
		args       args
		wantPath   string // Expected returned path
		wantErr    bool
		wantErrMsg string // Check error content
	}{
		{
			name:     "valid_filename",
			args:     args{path: "test.txt"},
			wantPath: "test.txt", // Expect cleaned path on success
			wantErr:  false,
		},
		{
			name:     "already_has_current_directory_prefix",
			args:     args{path: "./test.txt"},
			wantPath: "test.txt", // Expect cleaned path on success
			wantErr:  false,
		},
		{
			name:       "attempt_directory_traversal",
			args:       args{path: "../test.txt"},
			wantPath:   "../test.txt", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "contains '..'", // Updated error message
		},
		{
			name:       "attempt_absolute_path",
			args:       args{path: "/etc/passwd"},
			wantPath:   "/etc/passwd", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "must be a filename only", // Updated error message
		},
		{
			name:       "attempt_nested_directory",
			args:       args{path: "subdir/test.txt"},
			wantPath:   "subdir/test.txt", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "must be a filename only", // Updated error message
		},
		{
			name:       "attempt_double_dot_hidden_directory",
			args:       args{path: "..hidden/test.txt"}, // Cleaned path is ..hidden/test.txt
			wantPath:   "..hidden/test.txt",             // Expect original path on error
			wantErr:    true,
			wantErrMsg: "must be a filename only", // Base name check catches this
		},
		{
			name:       "attempt_double_dot_dir_itself",
			args:       args{path: ".."}, // Cleaned path is ..
			wantPath:   "..",             // Expect original path on error
			wantErr:    true,
			wantErrMsg: "contains '..'", // Contains ".." check catches this
		},
		{
			name:     "clean_path_with_dots",
			args:     args{path: "./././test.txt"},
			wantPath: "test.txt", // Expect cleaned path on success
			wantErr:  false,
		},
		{
			name:     "filename_with_dots",
			args:     args{path: "test.file.with.dots.txt"},
			wantPath: "test.file.with.dots.txt", // Expect cleaned path on success
			wantErr:  false,
		},
		{
			name:       "empty_path",
			args:       args{path: ""},
			wantPath:   "", // Expect original empty path
			wantErr:    true,
			wantErrMsg: "filename cannot be empty", // Matches error
		},
		{
			name:     "filename_with_special_characters",
			args:     args{path: "test-file_123.txt"},
			wantPath: "test-file_123.txt", // Expect cleaned path on success
			wantErr:  false,
		},
		{
			name:       "command_injection_attempt",
			args:       args{path: "file.txt; rm -rf"},
			wantPath:   "file.txt; rm -rf", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "contains invalid characters", // Matches error check
		},
		{
			name:       "another_command_injection_attempt", // $ and ()
			args:       args{path: "$(cat /etc/passwd)"},
			wantPath:   "$(cat /etc/passwd)", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "invalid file path", // Matches error check
		},
		{
			name:       "backtick_command_injection",
			args:       args{path: "`echo hello`"},
			wantPath:   "`echo hello`", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "contains invalid characters", // Matches error check
		},
		{
			name:       "pipe_command_injection",
			args:       args{path: "file.txt | cat /etc/passwd"},
			wantPath:   "file.txt | cat /etc/passwd", // Expect original path on error
			wantErr:    true,
			wantErrMsg: "invalid file path", // Matches error check
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure maxFilenameLength constant is accessible or defined for tests if needed
			// For this function, it's used internally, so no need here.

			gotPath, err := ValidateFilePath(tt.args.path) // Renamed 'got' to 'gotPath' for clarity

			// Check error status
			if tt.wantErr {
				require.Error(t, err, "Expected an error, but got nil")
				// Check error message content if specified
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg, "Error message mismatch")
				}
			} else {
				require.NoError(t, err, "Expected no error, but got: %v", err)
			}

			// Check returned path
			// On error, it should return the *original* path.
			// On success, it should return the *cleaned* path.
			expectedPath := tt.wantPath
			assert.Equal(t, expectedPath, gotPath, "Returned path mismatch")
		})
	}
}
