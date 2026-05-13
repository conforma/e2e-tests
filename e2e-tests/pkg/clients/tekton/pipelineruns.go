package tekton

import (
	"context"
	"fmt"
	"time"

	"github.com/conforma/e2e-tests/e2e-tests/pkg/utils/tekton"
	g "github.com/onsi/ginkgo/v2"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (t *TektonController) CreatePipelineRun(pipelineRun *pipeline.PipelineRun, ns string) (*pipeline.PipelineRun, error) {
	return t.PipelineClient().TektonV1().PipelineRuns(ns).Create(context.Background(), pipelineRun, metav1.CreateOptions{})
}

func (t *TektonController) createAndWait(pr *pipeline.PipelineRun, namespace string, taskTimeout int) (*pipeline.PipelineRun, error) {
	pipelineRun, err := t.CreatePipelineRun(pr, namespace)
	if err != nil {
		return nil, err
	}
	g.GinkgoWriter.Printf("Creating Pipeline %q\n", pipelineRun.Name)
	return pipelineRun, waitUntil(t.CheckPipelineRunStarted(pipelineRun.Name, namespace), time.Duration(taskTimeout)*time.Second)
}

func (t *TektonController) RunPipeline(g tekton.PipelineRunGenerator, namespace string, taskTimeout int) (*pipeline.PipelineRun, error) {
	pr, err := g.Generate()
	if err != nil {
		return nil, err
	}
	pvcs := t.KubeInterface().CoreV1().PersistentVolumeClaims(pr.Namespace)
	for _, w := range pr.Spec.Workspaces {
		if w.PersistentVolumeClaim != nil {
			pvcName := w.PersistentVolumeClaim.ClaimName
			if _, err := pvcs.Get(context.Background(), pvcName, metav1.GetOptions{}); err != nil {
				if errors.IsNotFound(err) {
					if err := tekton.CreatePVC(pvcs, pvcName); err != nil {
						return nil, err
					}
				} else {
					return nil, err
				}
			}
		}
	}
	return t.createAndWait(pr, namespace, taskTimeout)
}

func (t *TektonController) GetPipelineRun(pipelineRunName, namespace string) (*pipeline.PipelineRun, error) {
	return t.PipelineClient().TektonV1().PipelineRuns(namespace).Get(context.Background(), pipelineRunName, metav1.GetOptions{})
}

func (t *TektonController) WatchPipelineRun(pipelineRunName, namespace string, taskTimeout int) error {
	g.GinkgoWriter.Printf("Waiting for pipeline %q to finish\n", pipelineRunName)
	return waitUntil(t.CheckPipelineRunFinished(pipelineRunName, namespace), time.Duration(taskTimeout)*time.Second)
}

func (t *TektonController) CheckPipelineRunStarted(pipelineRunName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pr, err := t.GetPipelineRun(pipelineRunName, namespace)
		if err != nil {
			return false, nil
		}
		return pr.Status.StartTime != nil, nil
	}
}

func (t *TektonController) CheckPipelineRunFinished(pipelineRunName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pr, err := t.GetPipelineRun(pipelineRunName, namespace)
		if err != nil {
			return false, nil
		}
		return pr.Status.CompletionTime != nil, nil
	}
}

func (t *TektonController) DeletePipelineRun(name, ns string) error {
	return t.PipelineClient().TektonV1().PipelineRuns(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
}

func (t *TektonController) GetPipelineRunLogs(prefix, pipelineRunName, namespace string) (string, error) {
	podClient := t.KubeInterface().CoreV1().Pods(namespace)
	podList, err := podClient.List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	podLog := ""
	for _, pod := range podList.Items {
		if len(pod.Name) < len(prefix) || pod.Name[:len(prefix)] != prefix {
			continue
		}
		for _, c := range pod.Spec.InitContainers {
			cLog, _ := t.fetchContainerLog(pod.Name, c.Name, namespace)
			podLog += fmt.Sprintf("\npod: %s | init container: %s\n%s", pod.Name, c.Name, cLog)
		}
		for _, c := range pod.Spec.Containers {
			cLog, _ := t.fetchContainerLog(pod.Name, c.Name, namespace)
			podLog += fmt.Sprintf("\npod: %s | container: %s\n%s", pod.Name, c.Name, cLog)
		}
	}
	return podLog, nil
}

func waitUntil(cond wait.ConditionFunc, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) { return cond() })
}
