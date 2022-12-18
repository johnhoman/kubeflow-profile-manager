package manager

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager interface {
	GetClient() client.Client
	GetScheme() *runtime.Scheme
}
