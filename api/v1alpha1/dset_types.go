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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DSetSpec defines the desired state of DSet
type DSetSpec struct {
	// External Load Balancer IP address

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15

	// +operator-sdk:csv:customresourcedefinitions:type=spec
	LoadBalancer string `json:"loadbalancer,omitempty"`
}

// DSetStatus defines the observed state of DSet
type DSetStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Represents the observations of a DSet's current state.
	// DSet.status.conditions.type are: "Available", "Progressing", and "Degraded"
	// DSet.status.conditions.status are one of True, False, Unknown.
	// DSet.status.conditions.reason the value should be a CamelCase string and producers of specific
	// condition types may define expected values and meanings for this field, and whether the values
	// are considered a guaranteed API.
	// DSet.status.conditions.Message is a human readable message indicating details about the transition.
	// For further information see: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	// Conditions store the status conditions of the DSet instances

	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DSet is the Schema for the dsets API
type DSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DSetSpec   `json:"spec,omitempty"`
	Status DSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DSetList contains a list of DSet
type DSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DSet{}, &DSetList{})
}
