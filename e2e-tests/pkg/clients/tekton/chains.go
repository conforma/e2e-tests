package tekton

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/conforma/e2e-tests/e2e-tests/pkg/utils/tekton"
	g "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var tektonChainsNamespaceCandidates = []string{
	"tekton-chains",
	"openshift-pipelines",
	"tekton-pipelines",
}

var (
	resolvedChainsNs string
	resolvedChainsMu sync.Mutex
)

func (t *TektonController) GetTektonChainsNamespace() (string, error) {
	resolvedChainsMu.Lock()
	defer resolvedChainsMu.Unlock()

	if resolvedChainsNs != "" {
		return resolvedChainsNs, nil
	}

	for _, ns := range tektonChainsNamespaceCandidates {
		pods, err := t.KubeInterface().CoreV1().Pods(ns).List(
			context.Background(), metav1.ListOptions{
				LabelSelector: "app=tekton-chains-controller",
			})
		if err != nil {
			continue
		}
		if len(pods.Items) > 0 {
			resolvedChainsNs = ns
			return resolvedChainsNs, nil
		}
	}
	return "", fmt.Errorf(
		"could not find tekton-chains-controller pods in any of: %v",
		tektonChainsNamespaceCandidates)
}

func (t *TektonController) GetTektonChainsPublicKey() ([]byte, error) {
	namespace, err := t.GetTektonChainsNamespace()
	if err != nil {
		return nil, err
	}

	secret, err := t.KubeInterface().CoreV1().Secrets(namespace).Get(context.Background(), "public-key", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("couldn't get the secret public-key from %s namespace: %+v", namespace, err)
	}
	publicKey := secret.Data["cosign.pub"]
	if len(publicKey) < 1 {
		return nil, fmt.Errorf("the content of cosign.pub in secret public-key in %s namespace is empty", namespace)
	}
	return publicKey, nil
}

func (t *TektonController) AwaitAttestationAndSignature(image string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (done bool, err error) {
		if _, err := tekton.FindCosignResultsForImage(image); err != nil {
			g.GinkgoWriter.Printf("failed to get cosign result for image %s: %+v\n", image, err)
			return false, nil
		}
		return true, nil
	})
}

func (t *TektonController) GetRekorHost() (string, error) {
	namespace, err := t.GetTektonChainsNamespace()
	if err != nil {
		return "", err
	}

	cm, err := t.KubeInterface().CoreV1().ConfigMaps(namespace).Get(context.Background(), "chains-config", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	rekorHost, ok := cm.Data["transparency.url"]
	if !ok || rekorHost == "" {
		rekorHost = "https://rekor.sigstore.dev"
	}
	return rekorHost, nil
}
