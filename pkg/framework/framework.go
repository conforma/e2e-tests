package framework

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/conforma/e2e-tests/pkg/clients/common"
	kubeCl "github.com/conforma/e2e-tests/pkg/clients/kubernetes"
	"github.com/conforma/e2e-tests/pkg/clients/tekton"
	"github.com/conforma/e2e-tests/pkg/constants"
	"github.com/devfile/library/v2/pkg/util"
	ginkgo "github.com/onsi/ginkgo/v2"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

type ControllerHub struct {
	CommonController *common.SuiteController
	TektonController *tekton.TektonController
}

type Framework struct {
	AsKubeAdmin   *ControllerHub
	UserNamespace string
	UserName      string
}

func NewFramework(userName string) (*Framework, error) {
	if userName == "" {
		return nil, fmt.Errorf("userName cannot be empty")
	}

	client, err := kubeCl.NewAdminKubernetesClient()
	if err != nil {
		return nil, err
	}

	asAdmin, err := InitControllerHub(client)
	if err != nil {
		return nil, fmt.Errorf("error initializing controllers: %v", err)
	}

	nsName := os.Getenv(constants.E2E_APPLICATIONS_NAMESPACE_ENV)
	if nsName == "" {
		nsName = userName
		_, err := asAdmin.CommonController.CreateTestNamespace(userName)
		if err != nil {
			return nil, fmt.Errorf("failed to create test namespace %s: %+v", nsName, err)
		}
	}

	return &Framework{
		AsKubeAdmin:   asAdmin,
		UserNamespace: nsName,
		UserName:      userName,
	}, nil
}

func InitControllerHub(cc *kubeCl.CustomClient) (*ControllerHub, error) {
	commonCtrl, err := common.NewSuiteController(cc)
	if err != nil {
		return nil, err
	}
	tektonController := tekton.NewSuiteController(cc)
	return &ControllerHub{
		CommonController: commonCtrl,
		TektonController: tektonController,
	}, nil
}

func ReportFailure(f **Framework) func() {
	return func() {
		if !ginkgo.CurrentSpecReport().Failed() {
			return
		}
		ginkgo.GinkgoWriter.Println("Test failed, collecting diagnostics...")
	}
}

func GetGeneratedNamespace(name string) string {
	return name + "-" + util.GenerateRandomString(4)
}

func GetQuayIOOrganization() string {
	org := os.Getenv(constants.QUAY_E2E_ORGANIZATION_ENV)
	if org == "" {
		return "redhat-appstudio-qe"
	}
	return org
}

func GetContainerLogs(ki kubernetes.Interface, podName, containerName, namespace string) (string, error) {
	req := ki.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{Container: containerName})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("error opening stream: %v", err)
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("error copying logs: %v", err)
	}
	return buf.String(), nil
}

func PrintTaskRunStatus(tr *pipeline.PipelineRunTaskRunStatus, namespace string, sc common.SuiteController) {
	if tr.Status == nil {
		ginkgo.GinkgoWriter.Println("*** TaskRun status: nil")
		return
	}
	if y, err := yaml.Marshal(tr.Status); err == nil {
		ginkgo.GinkgoWriter.Printf("*** TaskRun status:\n%s\n", string(y))
	} else {
		ginkgo.GinkgoWriter.Printf("*** Unable to serialize TaskRunStatus: %s\n", err)
	}
	for _, s := range tr.Status.Steps {
		if logs, err := GetContainerLogs(sc.KubeInterface(), tr.Status.PodName, s.Container, namespace); err == nil {
			ginkgo.GinkgoWriter.Printf("*** Logs from pod '%s', container '%s':\n----- START -----%s----- END -----\n", tr.Status.PodName, s.Container, logs)
		}
	}
}
