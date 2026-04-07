package workflow

import (
	"fmt"
	"os"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

// SharedJobs maps job key names to their definition.
// Keys must match JobID format (^[a-z][a-z0-9]*(_[a-z0-9]+)*$).
type SharedJobs map[string]*Job

type sharedFile struct {
	Jobs map[string]*Job `yaml:"jobs"`
}

// LoadShared parses a _shared.yaml file and returns the validated shared job definitions.
func LoadShared(path string) (SharedJobs, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening shared file: %w", err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var sf sharedFile
	if err := dec.Decode(&sf); err != nil {
		return nil, fmt.Errorf("parsing shared YAML: %w", err)
	}

	if len(sf.Jobs) == 0 {
		return nil, fmt.Errorf("shared file must contain at least one job")
	}

	for key := range sf.Jobs {
		if err := core.ValidateJobID(key); err != nil {
			return nil, fmt.Errorf("shared job key %q: %w", key, err)
		}
	}

	return SharedJobs(sf.Jobs), nil
}

// ResolveRef resolves a job's ref field against shared definitions using shallow merge semantics.
func ResolveRef(jobNode *yaml.Node, shared SharedJobs) (*Job, error) {
	refValue := getMappingValue(jobNode, "ref")
	if refValue == "" {
		return nil, fmt.Errorf("job node missing ref key")
	}

	sharedJob, ok := shared[refValue]
	if !ok {
		return nil, fmt.Errorf("ref %q not found in shared jobs: %w", refValue, core.ErrNotFound)
	}

	overlayKeys := getExplicitKeys(jobNode)

	result := *sharedJob

	if overlayKeys["id"] {
		result.ID = decodeMappingField(jobNode, "id")
	} else {
		result.ID = refValue
	}

	if overlayKeys["prompt"] {
		result.Prompt = decodeMappingField(jobNode, "prompt")
	}

	if overlayKeys["skill"] {
		result.Skill = decodeMappingField(jobNode, "skill")
	}

	if overlayKeys["description"] {
		result.Description = decodeMappingField(jobNode, "description")
	}

	result.Ref = refValue

	return &result, nil
}

func getMappingValue(node *yaml.Node, key string) string {
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value
		}
	}
	return ""
}

func getExplicitKeys(node *yaml.Node) map[string]bool {
	keys := make(map[string]bool, len(node.Content)/2)
	for i := 0; i < len(node.Content)-1; i += 2 {
		keys[node.Content[i].Value] = true
	}
	return keys
}

func decodeMappingField(node *yaml.Node, key string) string {
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			valNode := node.Content[i+1]
			if valNode.Tag == "!!null" || valNode.Kind == yaml.ScalarNode && valNode.Value == "" {
				return ""
			}
			return valNode.Value
		}
	}
	return ""
}
