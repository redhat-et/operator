package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FulcioSpec defines the desired state of Fulcio
type FulcioSpec struct {
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	// Ctlog service configuration
	//+optional
	//+kubebuilder:default:={port: 80, prefix: trusted-artifact-signer}
	Ctlog CtlogService `json:"ctlog,omitempty"`
	// Fulcio Configuration
	//+required
	Config FulcioConfig `json:"config"`
	// Certificate configuration
	Certificate FulcioCert `json:"certificate"`
	//Enable Service monitors for fulcio
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	// ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

// FulcioCert defines fields for system-generated certificate
// +kubebuilder:validation:XValidation:rule=(has(self.caRef) || self.organizationName != ""),message=organizationName cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.caRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type FulcioCert struct {
	// Reference to CA private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Reference to password to encrypt CA private key
	//+optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// Reference to CA certificate
	//+optional
	CARef *SecretKeySelector `json:"caRef,omitempty"`

	//+optional
	// CommonName specifies the common name for the Fulcio certificate.
	// If not provided, the common name will default to the host name.
	CommonName string `json:"commonName,omitempty"`
	//+optional
	OrganizationName string `json:"organizationName,omitempty"`
	//+optional
	OrganizationEmail string `json:"organizationEmail,omitempty"`
}

// FulcioConfig configuration of OIDC issuers
// +kubebuilder:validation:XValidation:rule=(has(self.OIDCIssuers) && (size(self.OIDCIssuers) > 0)) || (has(self.MetaIssuers) && (size(self.MetaIssuers) > 0)),message=At least one of OIDCIssuers or MetaIssuers must be defined
type FulcioConfig struct {
	// OIDC Configuration
	// +optional
	OIDCIssuers []OIDCIssuer `json:"OIDCIssuers,omitempty"`

	// A meta issuer has a templated URL of the form:
	//   https://oidc.eks.*.amazonaws.com/id/*
	// Where * can match a single hostname or URI path parts
	// (in particular, no '.' or '/' are permitted, among
	// other special characters)  Some examples we want to match:
	// * https://oidc.eks.us-west-2.amazonaws.com/id/B02C93B6A2D30341AD01E1B6D48164CB
	// * https://container.googleapis.com/v1/projects/mattmoor-credit/locations/us-west1-b/clusters/tenant-cluster
	// +optional
	MetaIssuers []OIDCIssuer `json:"MetaIssuers,omitempty"`
}

type OIDCIssuer struct {
	// The expected issuer of an OIDC token
	IssuerURL string `json:"IssuerURL,omitempty" yaml:"issuer-url,omitempty"`
	// The expected issuer of an OIDC token
	//+required
	Issuer string `json:"Issuer" yaml:"issuer"`
	//+required
	ClientID string `json:"ClientID" yaml:"client-id"`
	// Used to determine the subject of the certificate and if additional
	// certificate values are needed
	//+required
	Type string `json:"Type" yaml:"type"`
	// Optional, if the issuer is in a different claim in the OIDC token
	IssuerClaim string `json:"IssuerClaim,omitempty" yaml:"issuer-claim,omitempty"`
	// The domain that must be present in the subject for 'uri' issuer types
	// Also used to create an email for 'username' issuer types
	SubjectDomain string `json:"SubjectDomain,omitempty" yaml:"subject-domain,omitempty"`
	// SPIFFETrustDomain specifies the trust domain that 'spiffe' issuer types
	// issue ID tokens for. Tokens with a different trust domain will be
	// rejected.
	SPIFFETrustDomain string `json:"SPIFFETrustDomain,omitempty" yaml:"spiffe-trust-domain,omitempty"`
	// Optional, the challenge claim expected for the issuer
	// Set if using a custom issuer
	ChallengeClaim string `json:"ChallengeClaim,omitempty" yaml:"challenge-claim,omitempty"`
}

// FulcioStatus defines the observed state of Fulcio
type FulcioStatus struct {
	ServerConfigRef *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Certificate     *FulcioCert           `json:"certificate,omitempty"`
	Url             string                `json:"url,omitempty"`
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

// Fulcio is the Schema for the fulcios API
type Fulcio struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FulcioSpec   `json:"spec,omitempty"`
	Status FulcioStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FulcioList contains a list of Fulcio
type FulcioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Fulcio `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Fulcio{}, &FulcioList{})
}

func (i *Fulcio) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Fulcio) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Fulcio) GetTrustedCA() *LocalObjectReference {
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
