// SPDX-License-Identifier: MIT

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSimpleRef(t *testing.T) {
	type args struct {
		ref string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "valid_ref",
			args: args{ref: "master"},
			want: true,
		},
		{
			name: "valid_ref_v4",
			args: args{ref: "v4"},
			want: true,
		},
		{
			name: "full_sha",
			args: args{ref: "fc305205784a70b4cfc17397654f4c94e3153ce4"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSimpleRef(tt.args.ref); got != tt.want {
				t.Errorf("IsSimpleRef() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRefType(t *testing.T) {
	type args struct {
		ref string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "valid_ref",
			args: args{ref: "master"},
			want: "branch",
		},
		{
			name: "valid_ref_v4",
			args: args{ref: "v4"},
			want: "version",
		},
		{
			name: "full_sha",
			args: args{ref: "fc305205784a70b4cfc17397654f4c94e3153ce4"},
			want: "sha",
		},
		{
			name: "unknown",
			args: args{ref: ".${}"},
			want: "unknown",
		},
		{
			name: "empty",
			args: args{ref: ""},
			want: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetRefType(tt.args.ref); got != tt.want {
				t.Errorf("GetRefType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindAllActions(t *testing.T) {
	type args struct {
		workflow *Workflow
	}
	tests := []struct {
		name string
		args args
		want []WorkflowAction
	}{
		{
			name: "Workflow with standard step actions",
			args: args{
				workflow: &Workflow{
					Jobs: map[string]Job{
						"build": {
							Steps: []Step{
								{
									Name: "Check out code",
									Uses: "actions/checkout@v4", // Valid GitHub action
								},
								{
									Name: "Set up Go",
									Uses: "actions/setup-go@v5", // Valid GitHub action
								},
								{
									Name: "Run linter",
									Run:  "golangci-lint run", // No 'uses'
								},
							},
						},
						"test": {
							Steps: []Step{
								{
									Name: "Run tests",
									Uses: "actions/go-tester@v1.0.0", // Valid GitHub action
								},
							},
						},
					},
				},
			},
			// Expected output: a slice containing only the valid GitHub actions found
			want: []WorkflowAction{
				{Name: "actions", Repo: "checkout", Ref: "v4", Type: "github"},
				{Name: "actions", Repo: "setup-go", Ref: "v5", Type: "github"},
				{Name: "actions", Repo: "go-tester", Ref: "v1.0.0", Type: "github"},
			},
		},
		{
			name: "Workflow with mixed action types (job, step, local, docker, invalid)",
			args: args{
				workflow: &Workflow{
					Jobs: map[string]Job{
						"reusable_job": {
							Uses: "octo-org/example-repo/.github/workflows/reusable.yml@main", // Valid job-level action
						},
						"build_job": {
							Steps: []Step{
								{
									Name: "Valid Step Action",
									Uses: "actions/checkout@v3", // Valid GitHub action
								},
								{
									Name: "Local Action",
									Uses: "./.github/actions/my-local-action", // Local, should be ignored
								},
								{
									Name: "Docker Action",
									Uses: "docker://node:18-alpine", // Docker, should be ignored
								},
								{
									Name: "Invalid Action Reference",
									Uses: "just-a-repo-no-ref", // Invalid format, should be ignored (ParseActionReference fails)
								},
							},
						},
					},
				},
			},
			// Expected output: only the valid GitHub actions
			want: []WorkflowAction{
				// Repo includes the subpath for reusable workflows according to ParseActionReference logic
				{
					Name: "octo-org",
					Repo: "example-repo/.github/workflows/reusable.yml",
					Ref:  "main",
					Type: "github",
				},
				{Name: "actions", Repo: "checkout", Ref: "v3", Type: "github"},
				// Local, Docker, and Invalid actions are filtered out.
			},
		},
		{
			name: "Workflow with no actions",
			args: args{
				workflow: &Workflow{
					Jobs: map[string]Job{
						"job_without_uses": {
							Steps: []Step{
								{Run: "echo 'Hello'"},
							},
						},
					},
				},
			},
			// Expected output: an empty slice
			want: []WorkflowAction{}, // Or nil, depending on preference, empty slice is common
		},
		{
			name: "Nil workflow input",
			args: args{
				workflow: nil,
			},
			// Expected output: an empty slice (function handles nil input)
			want: []WorkflowAction{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindAllActions(tt.args.workflow)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
