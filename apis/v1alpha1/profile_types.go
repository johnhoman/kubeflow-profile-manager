/*
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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProfileCondition struct {
	Type    string `json:"type,omitempty"`
	Status  string `json:"status,omitempty" description:"status of the condition, one of True, False, Unknown"`
	Message string `json:"message,omitempty"`
}

// ProfileSpec defines the desired state of Profile
type ProfileSpec struct {
	// The profile owner
	Owner rbacv1.Subject `json:"owner"`

	// ResourceQuotaSpec that will be applied to target namespace
	ResourceQuotaSpec *corev1.ResourceQuotaSpec `json:"resourceQuotaSpec,omitempty"`
}

const (
	ProfileSucceed = "Successful"
	ProfileFailed  = "Failed"
	ProfileUnknown = "Unknown"
)

// ProfileStatus defines the observed state of Profile
type ProfileStatus struct {
	// Conditions
	Conditions []ProfileCondition `json:"conditions,omitempty"`

	// Contributors is a list of current contributors
	Contributors []corev1.LocalObjectReference `json:"contributors,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=profiles,scope=Cluster
// +kubebuilder:printcolumn:name="OWNER",type="string",JSONPath=".spec.owner.name"
// +kubebuilder:printcolumn:name="KIND",type="string",JSONPath=".spec.owner.kind"

// Profile is the Schema for the profiles API
type Profile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProfileSpec   `json:"spec,omitempty"`
	Status ProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProfileList contains a list of Profile
type ProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Profile `json:"items"`
}
