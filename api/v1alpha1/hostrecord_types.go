/*
Copyright 2025 nofy.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HostRecordSpec defines the desired state of HostRecord
type HostRecordSpec struct {
	// WAPIEndpoint is the fully-qualified base URL to the Infoblox WAPI endpoint, including version.
	// Example: https://infoblox.example.com/wapi/v2.12
	// +kubebuilder:validation:Pattern=`^https?://.+/wapi/v[0-9]+\.[0-9]+$`
	WAPIEndpoint string `json:"wapiEndpoint"`

	// CredentialsSecret references a Secret containing credentials to authenticate to Infoblox.
	// Required keys: "username" and "password".
	// If namespace is omitted, defaults to the namespace of the HostRecord resource.
	// +kubebuilder:validation:Required
	CredentialsSecret SecretReference `json:"credentialsSecret"`

	// InsecureTLS skips TLS verification when connecting to Infoblox.
	// +optional
	InsecureTLS bool `json:"insecureTLS,omitempty"`

	// FQDN is the DNS name of the host record to ensure exists in Infoblox.
	// +kubebuilder:validation:MinLength=1
	FQDN string `json:"fqdn"`

	// IP is the IPv4 address to assign. If omitted, an IP will be allocated from NetworkCIDR.
	// +optional
	IP string `json:"ip,omitempty"`

	// NetworkCIDR is the network from which to allocate the next available IP if IP is not provided.
	// Example: 10.0.0.0/24
	// +optional
	NetworkCIDR string `json:"networkCIDR,omitempty"`

	// DNSView is the DNS view in Infoblox (defaults to "default").
	// +kubebuilder:default=default
	// +optional
	DNSView string `json:"dnsView,omitempty"`

	// NetworkView is the network view for IP allocation (defaults to "default").
	// +kubebuilder:default=default
	// +optional
	NetworkView string `json:"networkView,omitempty"`

	// TTL is the desired TTL for the host record.
	// +kubebuilder:validation:Minimum=0
	// +optional
	TTL *int `json:"ttl,omitempty"`

	// ExtAttrs are Infoblox Extensible Attributes to set on the host record.
	// +optional
	ExtAttrs map[string]string `json:"extAttrs,omitempty"`
}

// HostRecordStatus defines the observed state of HostRecord.
type HostRecordStatus struct {
	// Ref is the Infoblox object reference for the created/managed host record.
	// +optional
	Ref string `json:"ref,omitempty"`

	// AllocatedIP is the IP address assigned to the host record.
	// +optional
	AllocatedIP string `json:"allocatedIP,omitempty"`

	// Conditions represent the latest available observations of an object's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HostRecord is the Schema for the hostrecords API
type HostRecord struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of HostRecord
	// +required
	Spec HostRecordSpec `json:"spec"`

	// status defines the observed state of HostRecord
	// +optional
	Status HostRecordStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// HostRecordList contains a list of HostRecord
type HostRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostRecord{}, &HostRecordList{})
}

// SecretReference identifies a Secret by name and optional namespace.
type SecretReference struct {
	// Name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the Secret. If omitted, defaults to the resource namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}
