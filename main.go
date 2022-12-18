package main

import (
	"github.com/alecthomas/kong"
	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

func init() {
	runtime.Must(v1alpha1.AddToScheme(scheme.Scheme))
}

var CLI struct{}

func main() {
	ctx := kong.Parse(&CLI, kong.Name("kubeflow-profile-manager"))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
		LeaderElectionID:       "profiles.kubeflow.org",
	})
	ctx.FatalIfErrorf(err, "unable to create  manager")
	ctx.FatalIfErrorf(mgr.Start(ctrl.SetupSignalHandler()), "failed to start manager")
}
