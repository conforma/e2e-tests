package client

import (
	ecp "github.com/conforma/crds/api/v1alpha1"
	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	tekton "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appstudioApi.AddToScheme(scheme))
	utilruntime.Must(tekton.AddToScheme(scheme))
	utilruntime.Must(ecp.AddToScheme(scheme))
}

type CustomClient struct {
	kubeClient     *kubernetes.Clientset
	crClient       crclient.Client
	pipelineClient pipelineclientset.Interface
	dynamicClient  dynamic.Interface
}

type K8SClient struct {
	AsKubeAdmin   *CustomClient
	UserName      string
	UserNamespace string
}

func (c *CustomClient) KubeInterface() kubernetes.Interface {
	return c.kubeClient
}

func (c *CustomClient) KubeRest() crclient.Client {
	return c.crClient
}

func (c *CustomClient) PipelineClient() pipelineclientset.Interface {
	return c.pipelineClient
}

func (c *CustomClient) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}

func NewAdminKubernetesClient() (*CustomClient, error) {
	adminKubeconfig, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return newClientFromConfig(adminKubeconfig)
}

func newClientFromConfig(cfg *rest.Config) (*CustomClient, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	pipelineClient, err := pipelineclientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	crClient, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &CustomClient{
		kubeClient:     kubeClient,
		crClient:       crClient,
		pipelineClient: pipelineClient,
		dynamicClient:  dynamicClient,
	}, nil
}
