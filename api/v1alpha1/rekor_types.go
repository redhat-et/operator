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
	//+kubebuilder:default:={size: "5Gi", retain: true, accessModes: {ReadWriteOnce}}
	Pvc Pvc `json:"pvc,omitempty"`
	// BackFillRedis CronJob Configuration
	//+kubebuilder:default:={enabled: true, schedule: "0 0 * * *"}
	BackFillRedis BackFillRedis `json:"backFillRedis,omitempty"`
	// Inactive shards
	// +listType=map
	// +listMapKey=treeID
	// +patchStrategy=merge
	// +patchMergeKey=treeID
	// +kubebuilder:default:={}
	Sharding []RekorLogRange `json:"sharding,omitempty"`
	// ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

type RekorSigner struct {

	// KMS Signer provider. Specifies the key management system (KMS) used for signing operations.
	//
	// Valid values:
	// - "secret" (default): The signer key is stored in a Kubernetes Secret.
	// - "memory": Ephemeral signer key stored in memory. Recommended for development use only.
	// - KMS URI: A URI to a cloud-based KMS, following the Go Cloud Development Kit (Go Cloud) URI format. Supported URIs include:
	//   - awskms://keyname
	//   - azurekms://keyname
	//   - gcpkms://keyname
	//   - hashivault://keyname
	// +kubebuilder:default:=secret
	KMS string `json:"kms,omitempty"`

	// Password to decrypt the signer private key.
	//
	// Optional field. This should be set only if the private key referenced by `keyRef` is encrypted with a password.
	// If KMS is set to a value other than "secret", this field is ignored.
	// +optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`

	// Reference to the signer private key.
	//
	// Optional field. When KMS is set to "secret", this field can be left empty, in which case the operator will automatically generate a signer key.
	// +optional
	KeyRef *SecretKeySelector `json:"keyRef,omitempty"`
}

type RekorSearchUI struct {
	// If set to true, the Operator will deploy a Rekor Search UI
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
	// Set Route Selector Labels labels for ingress sharding.
	RouteSelectorLabels map[string]string `json:"routeSelectorLabels,omitempty"`
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

// RekorLogRange defines the range and details of a log shard
// +structType=atomic
type RekorLogRange struct {
	// ID of Merkle tree in Trillian backend
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	TreeID int64 `json:"treeID"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// Length of the tree
	TreeLength int64 `json:"treeLength"`
	// The public key for the log shard, encoded in Base64 format
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9+/\n]+={0,2}\n*$`
	EncodedPublicKey string `json:"encodedPublicKey,omitempty"`
}

// RekorStatus defines the observed state of Rekor
type RekorStatus struct {
	// Reference to secret with Rekor's signer public key.
	// Public key is automatically generated from signer private key.
	PublicKeyRef     *SecretKeySelector    `json:"publicKeyRef,omitempty"`
	ServerConfigRef  *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Signer           RekorSigner           `json:"signer,omitempty"`
	PvcName          string                `json:"pvcName,omitempty"`
	Url              string                `json:"url,omitempty"`
	RekorSearchUIUrl string                `json:"rekorSearchUIUrl,omitempty"`
	// The ID of a Trillian tree that stores the log data.
	// +kubebuilder:validation:Type=number
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

func (i *Rekor) GetTrustedCA() *LocalObjectReference {
	if i.Spec.TrustedCA != nil {
		return i.Spec.TrustedCA
	}

	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &LocalObjectReference{
			Name: v,
		}
	}

	return nil
}
