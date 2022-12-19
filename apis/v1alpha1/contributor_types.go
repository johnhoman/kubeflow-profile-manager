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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ContributorRoleContributor = "Contributor"
	ContributorRoleOwner       = "Owner"
)

// ContributorSpec defines the desired state of Profile
type ContributorSpec struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// ContributorStatus is the status of a contributor
type ContributorStatus struct{}

// Contributor is the Schema for the profiles API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Contributor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContributorSpec   `json:"spec,omitempty"`
	Status ContributorStatus `json:"status,omitempty"`
}

// ContributorList contains a list of Contributors
// +kubebuilder:object:root=true
type ContributorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Contributor `json:"items"`
}
