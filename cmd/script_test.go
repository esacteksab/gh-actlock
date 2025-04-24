// SPDX-License-Identifier: MIT
package cmd_test

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestScripts(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("Skipping testscript tests: GITHUB_TOKEN environment variable not set")
	}

	testscript.Run(t, testscript.Params{
		Dir:           "../testdata/script",
		UpdateScripts: false,
		Setup: func(env *testscript.Env) error {
			// 1. Pass GITHUB_TOKEN to the script environment
			env.Vars = append(env.Vars, "GITHUB_TOKEN="+token)
			if gocoverdir := os.Getenv("GOCOVERDIR"); gocoverdir != "" {
				env.Vars = append(env.Vars, "GOCOVERDIR="+gocoverdir)
			}

			return nil // Successful setup
		},
	})
}
