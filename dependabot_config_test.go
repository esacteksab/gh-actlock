package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type dependabotConfig struct {
	Updates []dependabotUpdate `yaml:"updates"`
}

type dependabotUpdate struct {
	PackageEcosystem string                `yaml:"package-ecosystem"`
	Labels           []string              `yaml:"labels"`
	Allow            []dependabotRule      `yaml:"allow"`
	Ignore           []dependabotRule      `yaml:"ignore"`
}

type dependabotRule struct {
	DependencyName string   `yaml:"dependency-name"`
	UpdateTypes    []string `yaml:"update-types"`
}

func TestDependabotDockerBaseImageLabels(t *testing.T) {
	contents, err := os.ReadFile(".github/dependabot.yml")
	if err != nil {
		t.Fatalf("read Dependabot config: %v", err)
	}

	var cfg dependabotConfig
	if err := yaml.Unmarshal(contents, &cfg); err != nil {
		t.Fatalf("parse Dependabot config: %v", err)
	}

	var dockerUpdate *dependabotUpdate
	for i := range cfg.Updates {
		if cfg.Updates[i].PackageEcosystem == "docker" {
			dockerUpdate = &cfg.Updates[i]
			break
		}
	}

	if dockerUpdate == nil {
		t.Fatal("docker package-ecosystem config not found")
	}

	assert.Contains(t, dockerUpdate.Labels, "docker")
	assert.NotContains(t, dockerUpdate.Labels, "go")

	assert.Contains(t, dockerUpdate.Allow, dependabotRule{DependencyName: "esacteksab/go"})

	foundIgnore := false
	for _, rule := range dockerUpdate.Ignore {
		if rule.DependencyName == "esacteksab/go" {
			assert.Contains(t, rule.UpdateTypes, "version-update:semver-minor")
			foundIgnore = true
		}
	}
	assert.True(t, foundIgnore, "the Docker Go base image should ignore semver-minor bumps")

	var gomodUpdate *dependabotUpdate
	for i := range cfg.Updates {
		if cfg.Updates[i].PackageEcosystem == "gomod" {
			gomodUpdate = &cfg.Updates[i]
			break
		}
	}
	if assert.NotNil(t, gomodUpdate) {
		assert.Contains(t, gomodUpdate.Labels, "go")
	}
}
