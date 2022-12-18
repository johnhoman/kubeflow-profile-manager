package manager

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FromClient(cli client.Client) *manager {
	return &manager{cli: cli}
}

type manager struct {
	cli client.Client
}

func (m *manager) GetClient() client.Client   { return m.cli }
func (m *manager) GetScheme() *runtime.Scheme { return m.cli.Scheme() }

var _ Manager = &manager{}
