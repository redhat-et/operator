package v1alpha1

import (
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
	//+kubebuilder:default:=80
	//+optional
	Port *int32 `json:"port,omitempty"`
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
}

// TLSCert defines fields for TLS certificate
// +kubebuilder:validation:XValidation:rule=(!has(self.certRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type TLSCert struct {
	// Reference to the private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Reference to service certificate
	//+optional
	CertRef *SecretKeySelector `json:"certRef,omitempty"`
	// Reference to CA certificate
	//+optional
	CACertRef *LocalObjectReference `json:"caCertRef,omitempty"`
}
