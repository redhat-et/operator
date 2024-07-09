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

// TrillianSpec defines the desired state of Trillian
type TrillianSpec struct {
	// Define your database connection
	//+kubebuilder:validation:XValidation:rule=((!self.create && self.databaseSecretRef != null) || self.create),message=databaseSecretRef cannot be empty
	//+kubebuilder:default:={create: true, pvc: {size: "5Gi", retain: true}}
	Db TrillianDB `json:"database,omitempty"`
	//+optional
	TrillianServer TrillianServer `json:"server,omitempty"`
	//+optional
	TrillianSigner TrillianSigner `json:"signer,omitempty"`
	// Enable Monitoring for Logsigner and Logserver
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
}

type TrillianDB struct {
	// Create Database if a database is not created one must be defined using the DatabaseSecret field
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Create *bool `json:"create"`
	// Secret with values to be used to connect to an existing DB or to be used with the creation of a new DB
	// mysql-host: The host of the MySQL server
	// mysql-port: The port of the MySQL server
	// mysql-user: The user to connect to the MySQL server
	// mysql-password: The password to connect to the MySQL server
	// mysql-database: The database to connect to
	//+optional
	DatabaseSecretRef *LocalObjectReference `json:"databaseSecretRef,omitempty"`
	// PVC configuration
	//+kubebuilder:default:={size: "5Gi", retain: true}
	Pvc Pvc `json:"pvc,omitempty"`
}

type TrillianServer struct {
	// Secret with TLS server certificate, private key and CA certificate
	TLSCertificate TLSCert `json:"tls"`
}
type TrillianSigner struct {
	// Secret with TLS server certificate, private key and CA certificate
	TLSCertificate TLSCert `json:"tls"`
}

// TrillianStatus defines the observed state of Trillian
type TrillianStatus struct {
	Db TrillianDB `json:"database,omitempty"`
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

// Trillian is the Schema for the trillians API
type Trillian struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrillianSpec   `json:"spec,omitempty"`
	Status TrillianStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TrillianList contains a list of Trillian
type TrillianList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Trillian `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Trillian{}, &TrillianList{})
}

func (i *Trillian) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Trillian) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}
