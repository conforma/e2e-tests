package tekton

import (
	"context"
	"time"

	ecp "github.com/conforma/crds/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const ecPolicyName = "ec-policy"

func (t *TektonController) CreateEnterpriseContractPolicy(name, namespace string, ecpolicy ecp.EnterpriseContractPolicySpec) (*ecp.EnterpriseContractPolicy, error) {
	ec := &ecp.EnterpriseContractPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ecpolicy,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return ec, t.KubeRest().Create(ctx, ec)
}

func (t *TektonController) CreateOrUpdatePolicyConfiguration(namespace string, policy ecp.EnterpriseContractPolicySpec) error {
	ecPolicy := ecp.EnterpriseContractPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ecPolicyName,
			Namespace: namespace,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := t.KubeRest().Get(ctx, crclient.ObjectKey{
		Namespace: namespace,
		Name:      ecPolicyName,
	}, &ecPolicy)

	exists := true
	if err != nil {
		if errors.IsNotFound(err) {
			exists = false
		} else {
			return err
		}
	}

	ecPolicy.Spec = policy
	if !exists {
		return t.KubeRest().Create(ctx, &ecPolicy)
	}
	return t.KubeRest().Update(ctx, &ecPolicy)
}

func (t *TektonController) GetEnterpriseContractPolicy(name, namespace string) (*ecp.EnterpriseContractPolicy, error) {
	ecPolicy := ecp.EnterpriseContractPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := t.KubeRest().Get(ctx, crclient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &ecPolicy)
	return &ecPolicy, err
}
