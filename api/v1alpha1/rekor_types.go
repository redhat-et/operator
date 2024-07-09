package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RekorSpec defines the desired state of Rekor
type RekorSpec struct {
	// ID of Merkle tree in Trillian backend
	// If it is unset, the operator will create new Merkle tree in the Trillian backend
	//+optional
	TreeID *int64 `json:"treeID,omitempty"`
	// Trillian service configuration
	//+kubebuilder:default:={port: 8091}
	Trillian TrillianService `json:"trillian,omitempty"`
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//Enable Service monitors for rekor
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	// Rekor Search UI
	//+kubebuilder:default:={enabled: true}
	RekorSearchUI RekorSearchUI `json:"rekorSearchUI,omitempty"`
	// Signer configuration
	Signer RekorSigner `json:"signer,omitempty"`
	// PVC configuration
	//+kubebuilder:default:={size: "5Gi", retain: true}
	Pvc Pvc `json:"pvc,omitempty"`
	// BackFillRedis CronJob Configuration
	//+kubebuilder:default:={enabled: true, schedule: "0 0 * * *"}
	BackFillRedis BackFillRedis `json:"backFillRedis,omitempty"`
	// Reference to TLS server certificate, private key and CA certificate
	//+optional
	TLSCertificate TLSCert `json:"tls"`
}

type RekorSigner struct {
	// KMS Signer provider. Valid options are secret, memory or any supported KMS provider defined by go-cloud style URI
	//+kubebuilder:default:=secret
	KMS string `json:"kms,omitempty"`

	// Password to decrypt signer private key
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	// Reference to signer private key
	//+optional
	KeyRef *SecretKeySelector `json:"keyRef,omitempty"`
}

type RekorSearchUI struct {
	// If set to true, the Operator will deploy a Rekor Search UI
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
}

type BackFillRedis struct {
	//Enable the BackFillRedis CronJob
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	//Schedule for the BackFillRedis CronJob
	//+kubebuilder:default:="0 0 * * *"
	//+kubebuilder:validation:Pattern:="^(@(?i)(yearly|annually|monthly|weekly|daily|hourly)|((\\*(\\/[1-9][0-9]*)?|[0-9,-]+)+\\s){4}(\\*(\\/[1-9][0-9]*)?|[0-9,-]+)+)$"
	Schedule string `json:"schedule,omitempty"`
}

// RekorStatus defines the observed state of Rekor
type RekorStatus struct {
	ServerConfigRef  *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Signer           RekorSigner           `json:"signer,omitempty"`
	PvcName          string                `json:"pvcName,omitempty"`
	Url              string                `json:"url,omitempty"`
	RekorSearchUIUrl string                `json:"rekorSearchUIUrl,omitempty"`
	// The ID of a Trillian tree that stores the log data.
	TreeID *int64 `json:"treeID,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="The component status"
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,description="The component url"

// Rekor is the Schema for the rekors API
type Rekor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RekorSpec   `json:"spec,omitempty"`
	Status RekorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RekorList contains a list of Rekor
type RekorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rekor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rekor{}, &RekorList{})
}

func (i *Rekor) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Rekor) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}
