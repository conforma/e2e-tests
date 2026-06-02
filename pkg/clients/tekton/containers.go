package tekton

import (
	"context"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const containerLogTimeout = 2 * time.Minute
const containerLogMaxBytes = 10 * 1024 * 1024 // 10 MB

func (t *TektonController) fetchContainerLog(podName, containerName, namespace string) (string, error) {
	podClient := t.KubeInterface().CoreV1().Pods(namespace)
	req := podClient.GetLogs(podName, &corev1.PodLogOptions{Container: containerName})
	ctx, cancel := context.WithTimeout(context.Background(), containerLogTimeout)
	defer cancel()
	readCloser, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer readCloser.Close()
	b, err := io.ReadAll(io.LimitReader(readCloser, containerLogMaxBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
