package main

import (
	"github.com/alecthomas/kong"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/johnhoman/kubeflow-profile-manager/controller/contributor"
	"github.com/johnhoman/kubeflow-profile-manager/controller/features"
	"github.com/johnhoman/kubeflow-profile-manager/controller/profile"
	"go.uber.org/zap/zapcore"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
)

var CLI struct {
	MetricsBindAddress     string            `default:":8080"`
	HealthProbeBindAddress string            `default:":8081"`
	ClusterAdmin           []string          `help:"cluster admin"`
	UserIDHeader           string            `name:"userid-header" default:"kubeflow-userid"`
	UserIDPrefix           string            `name:"userid-prefix"`
	NamespaceLabels        map[string]string `help:"default labels to add to namespaces"`
	Debug                  bool              `help:"enable debug logging"`

	LeaderElect bool `name:"leader-elect" help:"enable leader election"`

	EnabledIstio    bool `name:"enable-istio" help:"enable integration with Istio" default:"true"`
	EnablePipelines bool `name:"enable-pipelines"`
}

func main() {
	ctx := kong.Parse(&CLI,
		kong.Name("Kubeflow Access Management"),
		kong.Description("User access management API server"),
		kong.DefaultEnvars(""),
	)
	ctx.FatalIfErrorf(v1alpha1.AddToScheme(scheme.Scheme))
	ctx.FatalIfErrorf(istiosecurity.AddToScheme(scheme.Scheme))

	flags := &feature.Flags{}
	if CLI.EnabledIstio {
		flags.Enable(features.Istio)
	}
	if CLI.EnablePipelines {
		flags.Enable(features.Pipelines)
	}

	zapLogger := zap.New(zap.UseDevMode(CLI.Debug), func(o *zap.Options) {
		o.TimeEncoder = zapcore.RFC3339TimeEncoder
	})

	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		Scheme:                 scheme.Scheme,
		Logger:                 zapLogger,
		LeaderElectionID:       "manager.profiles.kubeflow.org",
		LeaderElection:         CLI.LeaderElect,
		HealthProbeBindAddress: CLI.HealthProbeBindAddress,
		MetricsBindAddress:     CLI.MetricsBindAddress,
	})
	ctx.FatalIfErrorf(err, "unable to create manager")

	opts := controller.Options{
		Features: flags,
		Logger:   logging.NewLogrLogger(zapLogger),
	}

	ctx.FatalIfErrorf(profile.Setup(mgr, opts), "failed to setup profile controller")
	ctx.FatalIfErrorf(contributor.Setup(mgr, opts,
		contributor.WithUserIDPrefix(CLI.UserIDPrefix),
		contributor.WithUserIDHeader(CLI.UserIDHeader)),
		"failed to setup profile controller")
	ctx.FatalIfErrorf(mgr.AddHealthzCheck("healthz", healthz.Ping), "failed to add healthcheck")
	ctx.FatalIfErrorf(mgr.AddReadyzCheck("readyz", healthz.Ping), "failed to add ready check")
	ctx.FatalIfErrorf(mgr.Start(signals.SetupSignalHandler()), "unable to start controller manager")
}
