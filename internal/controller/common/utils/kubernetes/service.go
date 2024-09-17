package kubernetes

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateService(namespace string, name string, portName string, port int, targetPort int32, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       portName,
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(port),
					TargetPort: intstr.FromInt32(targetPort),
				},
			},
		},
	}
}

func FindService(ctx context.Context, c client.Client, namespace string, labels map[string]string) (*corev1.Service, error) {

	list := &corev1.ServiceList{}

	err := c.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(labels))

	if err != nil {
		return nil, err
	}
	if len(list.Items) > 1 {
		return nil, errors.New("duplicate resource")
	}

	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    list.GetObjectKind().GroupVersionKind().Group,
		Resource: list.GetObjectKind().GroupVersionKind().Kind,
	}, "")
}
