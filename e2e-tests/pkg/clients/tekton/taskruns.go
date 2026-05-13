package tekton

import (
	"context"
	"fmt"
	"strings"

	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *TektonController) GetTaskRunFromPipelineRun(c crclient.Client, pr *pipeline.PipelineRun, pipelineTaskName string) (*pipeline.TaskRun, error) {
	for _, chr := range pr.Status.ChildReferences {
		if chr.PipelineTaskName != pipelineTaskName {
			continue
		}
		taskRun := &pipeline.TaskRun{}
		taskRunKey := types.NamespacedName{Namespace: pr.Namespace, Name: chr.Name}
		if err := c.Get(context.Background(), taskRunKey, taskRun); err != nil {
			return nil, err
		}
		return taskRun, nil
	}
	return nil, fmt.Errorf("task %q not found in PipelineRun %q/%q", pipelineTaskName, pr.Namespace, pr.Name)
}

func (t *TektonController) GetTaskRunResult(c crclient.Client, pr *pipeline.PipelineRun, pipelineTaskName, result string) (string, error) {
	taskRun, err := t.GetTaskRunFromPipelineRun(c, pr, pipelineTaskName)
	if err != nil {
		return "", err
	}
	for _, trResult := range taskRun.Status.Results {
		if trResult.Name == result {
			return strings.TrimSuffix(trResult.Value.StringVal, "\n"), nil
		}
	}
	return "", fmt.Errorf("result %q not found in TaskRuns of PipelineRun %s/%s", result, pr.Namespace, pr.Name)
}

func (t *TektonController) GetTaskRunStatus(c crclient.Client, pr *pipeline.PipelineRun, pipelineTaskName string) (*pipeline.PipelineRunTaskRunStatus, error) {
	for _, chr := range pr.Status.ChildReferences {
		if chr.PipelineTaskName == pipelineTaskName {
			taskRun := &pipeline.TaskRun{}
			taskRunKey := types.NamespacedName{Namespace: pr.Namespace, Name: chr.Name}
			if err := c.Get(context.Background(), taskRunKey, taskRun); err != nil {
				return nil, err
			}
			return &pipeline.PipelineRunTaskRunStatus{PipelineTaskName: chr.PipelineTaskName, Status: &taskRun.Status}, nil
		}
	}
	return nil, fmt.Errorf("TaskRun status for pipeline task name %q not found in PipelineRun %s/%s", pipelineTaskName, pr.Namespace, pr.Name)
}
