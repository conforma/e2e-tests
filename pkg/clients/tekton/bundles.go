package tekton

import (
	"context"
	"fmt"

	"github.com/conforma/e2e-tests/pkg/utils/tekton"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v2"
)

type Bundles struct {
	DockerBuildBundle                   string
	DockerBuildMultiPlatformOCITABundle string
	DockerBuildOCITABundle              string
	DockerBuildOCITAMinBundle           string
	FBCBuilderBundle                    string
}

func (t *TektonController) NewBundles() (*Bundles, error) {
	namespacedName := types.NamespacedName{
		Name:      "build-pipeline-config",
		Namespace: "build-service",
	}
	bundles := &Bundles{}
	configMap := &corev1.ConfigMap{}
	err := t.KubeRest().Get(context.Background(), namespacedName, configMap)
	if err != nil {
		return nil, err
	}

	bpc := &tekton.BuildPipelineConfig{}
	if err = yaml.Unmarshal([]byte(configMap.Data["config.yaml"]), bpc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal build pipeline config: %v", err)
	}

	for i := range bpc.Pipelines {
		p := bpc.Pipelines[i]
		switch p.Name {
		case "docker-build":
			bundles.DockerBuildBundle = p.Bundle
		case "docker-build-multi-platform-oci-ta":
			bundles.DockerBuildMultiPlatformOCITABundle = p.Bundle
		case "docker-build-oci-ta":
			bundles.DockerBuildOCITABundle = p.Bundle
		case "docker-build-oci-ta-min":
			bundles.DockerBuildOCITAMinBundle = p.Bundle
		case "fbc-builder":
			bundles.FBCBuilderBundle = p.Bundle
		}
	}
	return bundles, nil
}
