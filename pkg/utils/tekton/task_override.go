package tekton

import (
	"fmt"
	"os"
	"strings"

	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/yaml"
)

func LoadAndPatchTaskImage(yamlPath, newImage string) (*pipeline.EmbeddedTask, error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("reading task YAML %s: %w", yamlPath, err)
	}

	var task pipeline.Task
	if err := yaml.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("unmarshaling task YAML: %w", err)
	}

	patched := 0
	for i := range task.Spec.Steps {
		if strings.Contains(task.Spec.Steps[i].Image, "quay.io/conforma/cli") {
			task.Spec.Steps[i].Image = newImage
			patched++
		}
	}
	if patched == 0 {
		return nil, fmt.Errorf("no steps matched quay.io/conforma/cli in %s", yamlPath)
	}

	return &pipeline.EmbeddedTask{TaskSpec: task.Spec}, nil
}
