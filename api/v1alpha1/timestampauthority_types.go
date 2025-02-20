/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TimestampAuthoritySpec defines the desired state of TimestampAuthority
// +kubebuilder:validation:XValidation:rule=!(has(self.signer.certificateChain.certificateChainRef) && (has(self.signer.certificateChain.intermediateCA) || has(self.signer.certificateChain.leafCA) || has(self.signer.certificateChain.rootCA))),message="when certificateChainRef is set, intermediateCA, leafCA, and rootCA must not be set"
type TimestampAuthoritySpec struct {
	//Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//Signer configuration
	//+required
	Signer TimestampAuthoritySigner `json:"signer"`
	//Enable Service monitors for Timestamp Authority
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	//ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
	//Configuration for NTP monitoring
	//+optional
	NTPMonitoring NTPMonitoring `json:"ntpMonitoring,omitempty"`
}

// TimestampAuthoritySigner defines the desired state of the Timestamp Authority Signer
// +kubebuilder:validation:XValidation:rule=(!(has(self.file) || has(self.kms) || has(self.tink)) || has(self.certificateChain.certificateChainRef)),message="signer config needs a matching cert chain in certificateChain.certificateChainRef"
// +kubebuilder:validation:XValidation:rule=(has(self.file) || has(self.kms) || has(self.tink) || !has(self.certificateChain.certificateChainRef)),message="certificateChainRef should not be present if no signers are configured"
// +kubebuilder:validation:XValidation:rule=(!(has(self.file) && has(self.kms)) && !(has(self.file) && has(self.tink)) && !(has(self.kms) && has(self.tink))),message="only one signer should be configured at any time"
type TimestampAuthoritySigner struct {
	//Configuration for the Certificate Chain
	//+required
	CertificateChain CertificateChain `json:"certificateChain"`
	//Configuration for file-based signer
	//+optional
	File *File `json:"file,omitempty"`
	//Configuration for KMS based signer
	//+optional
	Kms *KMS `json:"kms,omitempty"`
	//Configuration for Tink based signer
	//+optional
	Tink *Tink `json:"tink,omitempty"`
}

// Certificate chain config
// +kubebuilder:validation:XValidation:rule="(!has(self.rootCA) && !has(self.leafCA)) || (has(self.rootCA.privateKeyRef) == has(self.leafCA.privateKeyRef))",message="must provide private keys for both root and leaf certificate authorities"
// +kubebuilder:validation:XValidation:rule=(has(self.certificateChainRef) || self.rootCA.organizationName != ""),message=organizationName cannot be empty for root certificate authority
// +kubebuilder:validation:XValidation:rule=(has(self.certificateChainRef) || self.leafCA.organizationName != ""),message=organizationName cannot be empty for leaf certificate authority
// +kubebuilder:validation:XValidation:rule=(has(self.certificateChainRef) || self.intermediateCA[0].organizationName != ""),message="organizationName cannot be empty for intermediate certificate authority, please make sure all are in place"
type CertificateChain struct {
	//Reference to the certificate chain
	//+optional
	CertificateChainRef *SecretKeySelector `json:"certificateChainRef,omitempty"`
	//Root Certificate Authority Config
	//+optional
	RootCA *TsaCertificateAuthority `json:"rootCA,omitempty"`
	//Intermediate Certificate Authority Config
	//+optional
	IntermediateCA []*TsaCertificateAuthority `json:"intermediateCA,omitempty"`
	//Leaf Certificate Authority Config
	//+optional
	LeafCA *TsaCertificateAuthority `json:"leafCA,omitempty"`
}

// TSA Certificate Authority configuration
type TsaCertificateAuthority struct {
	//CommonName specifies the common name for the TimeStampAuthorities cert chain.
	//If not provided, the common name will default to the host name.
	//+optional
	CommonName string `json:"commonName,omitempty"`
	//+optional
	//OrganizationName specifies the Organization Name for the TimeStampAuthorities cert chain.
	OrganizationName string `json:"organizationName,omitempty"`
	//+optional
	//Organization Email specifies the Organization Email for the TimeStampAuthorities cert chain.
	OrganizationEmail string `json:"organizationEmail,omitempty"`
	//Password to decrypt the signer's root private key
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	// Reference to the signer's root private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
}

// TSA File signer configuration
type File struct {
	//Password to decrypt the signer's root private key
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	//Reference to the signer's root private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
}

// TSA KMS signer config
type KMS struct {
	//KMS key for signing timestamp responses. Valid options include: [gcpkms://resource, azurekms://resource, hashivault://resource, awskms://resource]
	//+required
	KeyResource string `json:"keyResource,omitempty"`
	//Configuration for authentication for key management services
	//+optional
	Auth *Auth `json:"auth,omitempty"`
}

// TSA Tink signer config
type Tink struct {
	//KMS key for signing timestamp responses for Tink keysets. Valid options include: [gcp-kms://resource, aws-kms://resource, hcvault://]"
	//+required
	KeyResource string `json:"keyResource,omitempty"`
	//+required
	//Path to KMS-encrypted keyset for Tink, decrypted by TinkKeyResource
	KeysetRef *SecretKeySelector `json:"keysetRef,omitempty"`
	// Configuration for authentication for key management services
	//+optional
	Auth *Auth `json:"auth,omitempty"`
}

type NTPMonitoring struct {
	//Enable or disable NTP(Network Time Protocol) Monitoring, Enabled by default
	//+kubebuilder:default:=true
	Enabled bool `json:"enabled"`
	//Configuration for Network time protocol monitoring
	Config *NtpMonitoringConfig `json:"config,omitempty"`
}

type NtpMonitoringConfig struct {
	//ConfigMap containing YAML configuration for NTP monitoring
	//Default configuration: https://github.com/securesign/timestamp-authority/blob/main/pkg/ntpmonitor/ntpsync.yaml
	NtpConfigRef *LocalObjectReference `json:"ntpConfigRef,omitempty"`
	//Number of attempts to contact a ntp server before giving up.
	RequestAttempts int `json:"requestAttempts,omitempty"`
	//The timeout in seconds for a request to respond. This value must be
	//smaller than max_time_delta.
	RequestTimeout int `json:"requestTimeout,omitempty"`
	//Number of randomly selected ntp servers to interrogate.
	NumServers int `json:"numServers,omitempty"`
	//Maximum number of seconds the local time is allowed to drift from the
	//response of a ntp server
	MaxTimeDelta int `json:"maxTimeDelta,omitempty"`
	//Number of servers who must agree with local time.
	ServerThreshold int `json:"serverThreshold,omitempty"`
	//Period (in seconds) for polling ntp servers
	Period int `json:"period,omitempty"`
	//List of servers to contact. Many DNS names resolves to multiple A records.
	Servers []string `json:"servers,omitempty"`
}

func (i *TimestampAuthority) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *TimestampAuthority) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

// TimestampAuthorityStatus defines the observed state of TimestampAuthority
type TimestampAuthorityStatus struct {
	NTPMonitoring *NTPMonitoring            `json:"ntpMonitoring,omitempty"`
	Signer        *TimestampAuthoritySigner `json:"signer,omitempty"`
	Url           string                    `json:"url,omitempty"`
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

// TimestampAuthority is the Schema for the timestampauthorities API
type TimestampAuthority struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TimestampAuthoritySpec   `json:"spec,omitempty"`
	Status TimestampAuthorityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TimestampAuthorityList contains a list of TimestampAuthority
type TimestampAuthorityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TimestampAuthority `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TimestampAuthority{}, &TimestampAuthorityList{})
}

func (i *TimestampAuthority) GetTrustedCA() *LocalObjectReference {
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
