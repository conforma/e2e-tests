package common

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"time"

	kubeCl "github.com/conforma/e2e-tests/pkg/clients/kubernetes"
	"github.com/conforma/e2e-tests/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
)

type SuiteController struct {
	*kubeCl.CustomClient
}

func NewSuiteController(cc *kubeCl.CustomClient) (*SuiteController, error) {
	return &SuiteController{CustomClient: cc}, nil
}

// GetSecret returns a secret by name and namespace.
func (s *SuiteController) GetSecret(ns, name string) (*corev1.Secret, error) {
	return s.KubeInterface().CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
}

// GetConfigMap returns a configmap by name and namespace.
func (s *SuiteController) GetConfigMap(name, namespace string) (*corev1.ConfigMap, error) {
	return s.KubeInterface().CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// ListPods returns a list of pods from a namespace by labels.
func (s *SuiteController) ListPods(namespace, labelKey, labelValue string, selectionLimit int64) (*corev1.PodList, error) {
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{labelKey: labelValue}}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
		Limit:         selectionLimit,
	}
	return s.KubeInterface().CoreV1().Pods(namespace).List(context.Background(), listOptions)
}

// IsPodRunning returns a condition function that checks if a pod is running.
func (s *SuiteController) IsPodRunning(podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := s.KubeInterface().CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		switch pod.Status.Phase {
		case corev1.PodRunning:
			return true, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			return false, fmt.Errorf("pod %q ran to completion", pod.Name)
		}
		return false, nil
	}
}

// WaitForPodSelector waits for pods matching a label selector to be in the expected state.
func (s *SuiteController) WaitForPodSelector(
	fn func(podName, namespace string) wait.ConditionFunc,
	namespace, labelKey, labelValue string,
	timeout int, selectionLimit int64) error {
	podList, err := s.ListPods(namespace, labelKey, labelValue, selectionLimit)
	if err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods in %s with label key %s and label value %s", namespace, labelKey, labelValue)
	}
	for i := range podList.Items {
		if err := waitUntil(fn(podList.Items[i].Name, namespace), time.Duration(timeout)*time.Second); err != nil {
			return err
		}
	}
	return nil
}

// CreateQuayRegistrySecret copies the quay secret to a user-defined namespace.
func (s *SuiteController) CreateQuayRegistrySecret(namespace string) error {
	var dockerConfigJsonData []byte
	sharedSecret, err := s.GetSecret(constants.QuayRepositorySecretNamespace, constants.QuayRepositorySecretName)
	if err != nil {
		quayToken := os.Getenv("QUAY_TOKEN")
		if quayToken == "" {
			return fmt.Errorf("failed to obtain quay token from 'QUAY_TOKEN' env; make sure the env var exists")
		}
		decoded, decErr := base64.StdEncoding.DecodeString(quayToken)
		if decErr == nil && json.Valid(decoded) {
			dockerConfigJsonData = decoded
		} else if json.Valid([]byte(quayToken)) {
			dockerConfigJsonData = []byte(quayToken)
		} else {
			return fmt.Errorf("QUAY_TOKEN is not valid docker config JSON (either raw or base64-encoded)")
		}
	} else {
		dockerConfigJsonData = sharedSecret.Data[".dockerconfigjson"]
	}

	_, err = s.GetSecret(namespace, constants.QuayRepositorySecretName)
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return err
		}
	} else {
		if err = s.KubeInterface().CoreV1().Secrets(namespace).Delete(context.Background(), constants.QuayRepositorySecretName, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	repositorySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: constants.QuayRepositorySecretName, Namespace: namespace},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigJsonData},
	}
	_, err = s.KubeInterface().CoreV1().Secrets(namespace).Create(context.Background(), repositorySecret, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return s.LinkSecretToServiceAccount(namespace, constants.QuayRepositorySecretName, constants.DefaultPipelineServiceAccount, true)
}

// LinkSecretToServiceAccount links a secret to a service account.
func (s *SuiteController) LinkSecretToServiceAccount(ns, secret, serviceaccount string, addImagePullSecrets bool) error {
	timeout := 20 * time.Second
	return wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		sa, err := s.KubeInterface().CoreV1().ServiceAccounts(ns).Get(context.Background(), serviceaccount, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, ref := range sa.Secrets {
			if ref.Name == secret {
				return true, nil
			}
		}
		sa.Secrets = append(sa.Secrets, corev1.ObjectReference{Name: secret})
		if addImagePullSecrets {
			sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: secret})
		}
		_, err = s.KubeInterface().CoreV1().ServiceAccounts(ns).Update(context.Background(), sa, metav1.UpdateOptions{})
		if err != nil {
			return false, nil
		}
		return true, nil
	})
}

// CreateTestNamespace creates a test namespace with required labels.
func (s *SuiteController) CreateTestNamespace(name string) (*corev1.Namespace, error) {
	ns, err := s.KubeInterface().CoreV1().Namespaces().Get(context.Background(), name, metav1.GetOptions{})
	requiredLabels := map[string]string{
		constants.ArgoCDLabelKey:    constants.ArgoCDLabelValue,
		constants.TenantLabelKey:    constants.TenantLabelValue,
		constants.WorkspaceLabelKey: name,
	}

	if err != nil {
		if k8sErrors.IsNotFound(err) {
			nsTemplate := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   name,
					Labels: requiredLabels,
				},
			}
			ns, err = s.KubeInterface().CoreV1().Namespaces().Create(context.Background(), &nsTemplate, metav1.CreateOptions{})
			if err != nil {
				return nil, fmt.Errorf("error when creating %s namespace: %v", name, err)
			}
			err = waitUntil(func() (bool, error) {
				fetchedNs, err := s.KubeInterface().CoreV1().Namespaces().Get(context.Background(), name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				return fetchedNs.Status.Phase == corev1.NamespaceActive, nil
			}, 30*time.Second)
			if err != nil {
				return nil, fmt.Errorf("timeout waiting for namespace %s to be ready: %v", name, err)
			}
		} else {
			return nil, fmt.Errorf("error when getting the '%s' namespace: %v", name, err)
		}
	} else {
		updated := ensureLabelsExist(ns, requiredLabels)
		if updated {
			ns, err = s.KubeInterface().CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
			if err != nil {
				return nil, fmt.Errorf("error when updating labels in '%s' namespace: %v", name, err)
			}
		} else {
			return ns, nil
		}
	}

	_, err = s.KubeInterface().CoreV1().ServiceAccounts(name).Get(context.Background(), constants.DefaultPipelineServiceAccount, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: constants.DefaultPipelineServiceAccount, Namespace: name},
			}
			_, err = s.KubeInterface().CoreV1().ServiceAccounts(name).Create(context.Background(), sa, metav1.CreateOptions{})
			if err != nil && !k8sErrors.IsAlreadyExists(err) {
				return nil, fmt.Errorf("error creating service account %s in namespace %s: %v", constants.DefaultPipelineServiceAccount, name, err)
			}
		} else {
			return nil, fmt.Errorf("error getting service account %s in namespace %s: %v", constants.DefaultPipelineServiceAccount, name, err)
		}
	}

	if os.Getenv(constants.TEST_ENVIRONMENT_ENV) == constants.UpstreamTestEnvironment {
		_, err = s.KubeInterface().RbacV1().RoleBindings(name).Get(context.Background(), constants.DefaultKonfluxAdminRoleBindingName, metav1.GetOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				rb := rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{Name: constants.DefaultKonfluxAdminRoleBindingName},
					Subjects:   []rbacv1.Subject{{Kind: "User", Name: constants.DefaultKonfluxCIUserName}},
					RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: constants.KonfluxAdminUserActionsClusterRoleName},
				}
				_, err = s.KubeInterface().RbacV1().RoleBindings(name).Create(context.Background(), &rb, metav1.CreateOptions{})
				if err != nil && !k8sErrors.IsAlreadyExists(err) {
					return nil, fmt.Errorf("error creating %s roleBinding: %v", constants.DefaultKonfluxAdminRoleBindingName, err)
				}
			} else {
				return nil, fmt.Errorf("error checking %s roleBinding: %v", constants.DefaultKonfluxAdminRoleBindingName, err)
			}
		}

		_, err = s.KubeInterface().RbacV1().RoleBindings(name).Get(context.Background(), constants.DefaultPipelineSARoleBindingName, metav1.GetOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				rb := rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{Name: constants.DefaultPipelineSARoleBindingName},
					Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: constants.DefaultPipelineServiceAccount, Namespace: name}},
					RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: constants.KonfluxAdminUserActionsClusterRoleName, APIGroup: "rbac.authorization.k8s.io"},
				}
				_, err = s.KubeInterface().RbacV1().RoleBindings(name).Create(context.Background(), &rb, metav1.CreateOptions{})
				if err != nil && !k8sErrors.IsAlreadyExists(err) {
					return nil, fmt.Errorf("error creating %s roleBinding: %v", constants.DefaultPipelineSARoleBindingName, err)
				}
			} else {
				return nil, fmt.Errorf("error checking %s roleBinding: %v", constants.DefaultPipelineSARoleBindingName, err)
			}
		}
	}
	return ns, nil
}

func ensureLabelsExist(ns *corev1.Namespace, requiredLabels map[string]string) bool {
	maps.DeleteFunc(requiredLabels, func(k, v string) bool {
		existing, ok := ns.Labels[k]
		return ok && existing == v
	})
	if len(requiredLabels) == 0 {
		return false
	}
	maps.Copy(ns.Labels, requiredLabels)
	return true
}

func waitUntil(cond wait.ConditionFunc, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) { return cond() })
}
