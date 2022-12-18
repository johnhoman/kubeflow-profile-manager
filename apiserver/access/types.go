package access

import rbacv1 "k8s.io/api/rbac/v1"

// Binding will give user edit access to referredNamespace
type Binding struct {
	User *rbacv1.Subject `json:"user,omitempty"`

	ReferredNamespace string `json:"referredNamespace,omitempty"`

	RoleRef *rbacv1.RoleRef `json:"RoleRef,omitempty"`

	// Status of the profile, one of Succeeded, Failed, Unknown.
	Status string `json:"status,omitempty"`
}
