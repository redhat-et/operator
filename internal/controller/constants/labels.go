package constants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelNamespace = "rhtas.redhat.com"
)

func LabelsFor(component, name, instance string) map[string]string {
	labels := LabelsForComponent(component, instance)
	labels["app.kubernetes.io/name"] = name

	return labels
}

func LabelsForComponent(component, instance string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":   instance,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/part-of":    "trusted-artifact-signer",
		"app.kubernetes.io/managed-by": "controller-manager",
	}
}

func LabelsRHTAS() map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":    "trusted-artifact-signer",
		"app.kubernetes.io/managed-by": "controller-manager",
	}
}

func AddLabel(ctx context.Context, object *metav1.PartialObjectMetadata, c client.Client, label string, value string) error {
	object.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})
	patch, err := json.Marshal([]map[string]any{
		{
			"op":   "add",
			"path": "/metadata/labels",
			"value": map[string]string{
				label: value,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %v", err)
	}

	err = c.Patch(ctx, object, client.RawPatch(types.JSONPatchType, patch))
	if err != nil {
		return fmt.Errorf("unable to add '%s' label to object: %w", label, err)
	}

	return nil
}

func RemoveLabel(ctx context.Context, object *metav1.PartialObjectMetadata, c client.Client, label string) error {
	object.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})
	patch, err := json.Marshal([]map[string]string{
		{
			"op":   "remove",
			"path": fmt.Sprintf("/metadata/labels/%s", strings.ReplaceAll(label, "/", "~1")),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %v", err)
	}

	err = c.Patch(ctx, object, client.RawPatch(types.JSONPatchType, patch))
	if err != nil {
		return fmt.Errorf("unable to remove '%s' label from secret: %w", label, err)
	}

	return nil
}
