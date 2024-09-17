package v1alpha1

import (
	core "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

type ExternalAccess struct {
	// If set to true, the Operator will create an Ingress or a Route resource.
	//For the plain Ingress there is no TLS configuration provided Route object uses "edge" termination by default.
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=false
	Enabled bool `json:"enabled"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
	// Set Route Selector Labels for ingress sharding.
	RouteSelectorLabels map[string]string `json:"routeSelectorLabels,omitempty"`
}

type MonitoringConfig struct {
	// If true, the Operator will create monitoring resources
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled bool `json:"enabled"`
}

// TrillianService configuration to connect Trillian server
type TrillianService struct {
	// Address to Trillian Log Server End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of Trillian Log Server End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+kubebuilder:default:=8091
	//+optional
	Port *int32 `json:"port,omitempty"`
}

// CtlogService configuration to connect Ctlog server
type CtlogService struct {
	// Address to Ctlog Log Server End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of Ctlog Log Server End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
	// Prefix is the name of the log. The prefix cannot be empty and can
	// contain "/" path separator characters to define global override handler prefix.
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9/]*[a-z0-9])?$"
	//+kubebuilder:default:=trusted-artifact-signer
	//+optional
	Prefix string `json:"prefix,omitempty"`
}

// LocalObjectReference contains enough information to let you locate the
// referenced object inside the same namespace.
// +structType=atomic
type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +required
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
}

// SecretKeySelector selects a key of a Secret.
// +structType=atomic
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key of the secret to select from. Must be a valid secret key.
	//+required
	//+kubebuilder:validation:Pattern:="^[-._a-zA-Z0-9]+$"
	Key string `json:"key" protobuf:"bytes,2,opt,name=key"`
}

// +kubebuilder:validation:Enum:=ReadWriteOnce;ReadOnlyMany;ReadWriteMany;ReadWriteOncePod
type PersistentVolumeAccessMode core.PersistentVolumeAccessMode

// Pvc configuration of the persistent storage claim for deployment in the cluster.
type Pvc struct {
	// The requested size of the persistent volume attached to Pod.
	// The format of this field matches that defined by kubernetes/apimachinery.
	// See https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity for more info on the format of this field.
	//+kubebuilder:default:="5Gi"
	Size *k8sresource.Quantity `json:"size,omitempty"`

	// Retain policy for the PVC
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Retain *bool `json:"retain"`
	// Name of the PVC
	//+optional
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`
	// The name of the StorageClass to claim a PersistentVolume from.
	//+optional
	StorageClass string `json:"storageClass,omitempty"`
	// PVC AccessModes
	//+kubebuilder:default:={ReadWriteOnce}
	//+kubebuilder:validation:MinItems:=1
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

type Auth struct {
	// Environmental variables used to define authentication parameters
	//+optional
	Env []core.EnvVar `json:"env,omitempty"`
	// Secret ref to be mounted inside a pod, Mount path defaults to /var/run/secrets/tas/auth
	//+optional
	SecretMount []SecretKeySelector `json:"secretMount,omitempty"`
}

// TLS (Transport Layer Security) Configuration for enabling service encryption.
// +kubebuilder:validation:XValidation:rule=(!has(self.certificateRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type TLS struct {
	// Reference to the private key secret used for TLS encryption.
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Reference to the certificate secret used for TLS encryption.
	//+optional
	CertRef *SecretKeySelector `json:"certificateRef,omitempty"`
}
