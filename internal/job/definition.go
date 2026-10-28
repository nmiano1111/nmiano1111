// Package job provides types and logic for multi-stage job definitions.
package job

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Definition is the top-level structure of a job YAML file.
type Definition struct {
	Name   string  `yaml:"name"`
	Runner string  `yaml:"runner"` // default runner for all stages
	Model  string  `yaml:"model"`  // default model for all stages
	Stages []Stage `yaml:"stages"`
}

// Stage describes a single unit of work within a job.
type Stage struct {
	ID        string   `yaml:"id"`
	Prompt    string   `yaml:"prompt"`
	DependsOn []string `yaml:"depends_on"`
	Agent     string   `yaml:"agent"`   // agent_id; defaults to stage ID
	Runner    string   `yaml:"runner"`  // overrides definition-level runner
	Model     string   `yaml:"model"`   // overrides definition-level model
	Timeout   float64  `yaml:"timeout"` // seconds; 0 = no timeout
}

// Load reads and parses a job definition from a YAML file.
func Load(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("job: read %s: %w", path, err)
	}
	var def Definition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("job: parse %s: %w", path, err)
	}
	return &def, nil
}

// Validate checks the definition for structural errors.
func (d *Definition) Validate() error {
	if len(d.Stages) == 0 {
		return fmt.Errorf("job has no stages")
	}

	ids := make(map[string]bool, len(d.Stages))
	for _, s := range d.Stages {
		if s.ID == "" {
			return fmt.Errorf("stage is missing an id")
		}
		if s.Prompt == "" {
			return fmt.Errorf("stage %q has no prompt", s.ID)
		}
		if ids[s.ID] {
			return fmt.Errorf("duplicate stage id %q", s.ID)
		}
		ids[s.ID] = true
	}

	// Check all depends_on references exist.
	for _, s := range d.Stages {
		for _, dep := range s.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("stage %q depends on unknown stage %q", s.ID, dep)
			}
		}
	}

	// Detect cycles via DFS.
	if err := detectCycles(d.Stages); err != nil {
		return err
	}

	// Check interpolation references resolve to known stage IDs.
	for _, s := range d.Stages {
		for _, ref := range interpolationRefs(s.Prompt) {
			if !ids[ref] {
				return fmt.Errorf("stage %q prompt references unknown stage %q", s.ID, ref)
			}
		}
	}

	return nil
}

// interpolationRefs returns the stage IDs referenced as {{id.result}} in text.
func interpolationRefs(prompt string) []string {
	var refs []string
	rest := prompt
	for {
		start := strings.Index(rest, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(rest[start:], "}}")
		if end == -1 {
			break
		}
		token := rest[start+2 : start+end]
		if strings.HasSuffix(token, ".result") {
			refs = append(refs, strings.TrimSuffix(token, ".result"))
		}
		rest = rest[start+end+2:]
	}
	return refs
}

// detectCycles uses DFS to find cycles in the dependency graph.
func detectCycles(stages []Stage) error {
	deps := make(map[string][]string, len(stages))
	for _, s := range stages {
		deps[s.ID] = s.DependsOn
	}

	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(stages))

	var dfs func(id string) error
	dfs = func(id string) error {
		if state[id] == visiting {
			return fmt.Errorf("cycle detected involving stage %q", id)
		}
		if state[id] == visited {
			return nil
		}
		state[id] = visiting
		for _, dep := range deps[id] {
			if err := dfs(dep); err != nil {
				return err
			}
		}
		state[id] = visited
		return nil
	}

	for _, s := range stages {
		if err := dfs(s.ID); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate replaces {{stage_id.result}} tokens in prompt with actual results.
func Interpolate(prompt string, results map[string]string) string {
	for id, result := range results {
		prompt = strings.ReplaceAll(prompt, "{{"+id+".result}}", result)
	}
	return prompt
}

// ReadyStages returns stages whose dependencies are all in the completed set
// and which are not already in the running or completed set.
func ReadyStages(stages []Stage, done, running map[string]bool) []Stage {
	var ready []Stage
	for _, s := range stages {
		if done[s.ID] || running[s.ID] {
			continue
		}
		allDone := true
		for _, dep := range s.DependsOn {
			if !done[dep] {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, s)
		}
	}
	return ready
}
