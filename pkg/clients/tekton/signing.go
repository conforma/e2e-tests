package tekton

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (t *TektonController) CreateOrUpdateSigningSecret(publicKey []byte, name, namespace string) (err error) {
	api := t.KubeInterface().CoreV1().Secrets(namespace)
	ctx := context.Background()

	expectedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       map[string][]byte{"cosign.pub": publicKey},
	}

	s, err := api.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}
		_, err = api.Create(ctx, expectedSecret, metav1.CreateOptions{})
		return
	}
	if string(s.Data["cosign.pub"]) != string(publicKey) {
		s.Data["cosign.pub"] = publicKey
		_, err = api.Update(ctx, s, metav1.UpdateOptions{})
	}
	return
}
