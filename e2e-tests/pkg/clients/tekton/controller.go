package tekton

import (
	kubeCl "github.com/conforma/e2e-tests/e2e-tests/pkg/clients/kubernetes"
)

type TektonController struct {
	*kubeCl.CustomClient
}

func NewSuiteController(kube *kubeCl.CustomClient) *TektonController {
	return &TektonController{kube}
}
