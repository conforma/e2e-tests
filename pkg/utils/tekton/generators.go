package tekton

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/conforma/e2e-tests/pkg/constants"
	app "github.com/konflux-ci/application-api/api/v1alpha1"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const sslCertDir = "/var/run/secrets/kubernetes.io/serviceaccount"

type PipelineRunGenerator interface {
	Generate() (*pipeline.PipelineRun, error)
}

type BuildahDemo struct {
	Image        string
	Bundle       string
	PipelineName string
	Name         string
	Namespace    string
}

type VerifyEnterpriseContract struct {
	Snapshot            app.SnapshotSpec
	TaskBundle          string
	TaskSpec            *pipeline.EmbeddedTask
	Name                string
	Namespace           string
	PolicyConfiguration string
	PublicKey           string
	Strict              bool
	EffectiveTime       string
	IgnoreRekor         bool
}

func (p *VerifyEnterpriseContract) WithComponentImage(imageRef string) {
	p.Snapshot.Components = []app.SnapshotComponent{
		{ContainerImage: imageRef},
	}
}

func (p *VerifyEnterpriseContract) AppendComponentImage(imageRef string) {
	p.Snapshot.Components = append(p.Snapshot.Components, app.SnapshotComponent{
		ContainerImage: imageRef,
	})
}

func (b BuildahDemo) Generate() (*pipeline.PipelineRun, error) {
	return &pipeline.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: b.Namespace,
		},
		Spec: pipeline.PipelineRunSpec{
			Params: []pipeline.Param{
				{Name: "dockerfile", Value: *pipeline.NewStructuredValues("Containerfile")},
				{Name: "output-image", Value: *pipeline.NewStructuredValues(b.Image)},
				{Name: "git-url", Value: *pipeline.NewStructuredValues("https://github.com/conforma/golden-container.git")},
				{Name: "skip-checks", Value: *pipeline.NewStructuredValues("true")},
			},
			PipelineRef: NewBundleResolverPipelineRef(b.PipelineName, b.Bundle),
			Workspaces: []pipeline.WorkspaceBinding{
				{
					Name: "workspace",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "app-studio-default-workspace",
					},
				},
			},
			TaskRunTemplate: pipeline.PipelineTaskRunTemplate{
				ServiceAccountName: constants.DefaultPipelineServiceAccount,
			},
		},
	}, nil
}

func (p VerifyEnterpriseContract) Generate() (*pipeline.PipelineRun, error) {
	applicationSnapshotJSON, err := json.Marshal(p.Snapshot)
	if err != nil {
		return nil, err
	}

	task := pipeline.PipelineTask{
		Name: "verify-enterprise-contract",
		Params: []pipeline.Param{
			{Name: "IMAGES", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: string(applicationSnapshotJSON)}},
			{Name: "POLICY_CONFIGURATION", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: p.PolicyConfiguration}},
			{Name: "PUBLIC_KEY", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: p.PublicKey}},
			{Name: "SSL_CERT_DIR", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: sslCertDir}},
			{Name: "STRICT", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: strconv.FormatBool(p.Strict)}},
			{Name: "EFFECTIVE_TIME", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: p.EffectiveTime}},
			{Name: "IGNORE_REKOR", Value: pipeline.ParamValue{Type: pipeline.ParamTypeString, StringVal: strconv.FormatBool(p.IgnoreRekor)}},
		},
	}

	if p.TaskSpec != nil {
		task.TaskSpec = p.TaskSpec
	} else {
		task.TaskRef = &pipeline.TaskRef{
			ResolverRef: pipeline.ResolverRef{
				Resolver: "bundles",
				Params: []pipeline.Param{
					{Name: "name", Value: pipeline.ParamValue{StringVal: "verify-enterprise-contract", Type: pipeline.ParamTypeString}},
					{Name: "bundle", Value: pipeline.ParamValue{StringVal: p.TaskBundle, Type: pipeline.ParamTypeString}},
					{Name: "kind", Value: pipeline.ParamValue{StringVal: "task", Type: pipeline.ParamTypeString}},
				},
			},
		}
	}

	return &pipeline.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-run-", p.Name),
			Namespace:    p.Namespace,
			Labels: map[string]string{
				"appstudio.openshift.io/application": p.Snapshot.Application,
			},
		},
		Spec: pipeline.PipelineRunSpec{
			PipelineSpec: &pipeline.PipelineSpec{
				Tasks: []pipeline.PipelineTask{task},
			},
			TaskRunTemplate: pipeline.PipelineTaskRunTemplate{
				ServiceAccountName: constants.DefaultPipelineServiceAccount,
			},
		},
	}, nil
}

func NewBundleResolverPipelineRef(name, bundleRef string) *pipeline.PipelineRef {
	return &pipeline.PipelineRef{
		ResolverRef: pipeline.ResolverRef{
			Resolver: "bundles",
			Params: []pipeline.Param{
				{Name: "name", Value: pipeline.ParamValue{StringVal: name, Type: pipeline.ParamTypeString}},
				{Name: "bundle", Value: pipeline.ParamValue{StringVal: bundleRef, Type: pipeline.ParamTypeString}},
				{Name: "kind", Value: pipeline.ParamValue{StringVal: "pipeline", Type: pipeline.ParamTypeString}},
			},
		},
	}
}

type BuildPipelineConfig struct {
	DefaultPipelineName string        `json:"default-pipeline-name"`
	Pipelines           []PipelineRef `json:"pipelines"`
}

type PipelineRef struct {
	Name   string `json:"name"`
	Bundle string `json:"bundle"`
}

func CreatePVC(pvcs v1.PersistentVolumeClaimInterface, pvcName string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: pvcName},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
	_, err := pvcs.Create(context.Background(), pvc, metav1.CreateOptions{})
	return err
}
