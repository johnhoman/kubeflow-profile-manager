package main

import (
	"context"

	"github.com/alecthomas/kong"
	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"github.com/johnhoman/kubeflow-profile-manager/apiserver"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	CLI struct {
		ClusterAdmin []string `help:"cluster admin"`
		UserIDHeader string   `name:"userid-header" default:"kubeflow-userid"`
		UserIDPrefix string   `name:"userid-prefix"`
	}
)

func init() {
	utilruntime.Must(v1alpha1.AddToScheme(scheme.Scheme))
}

func main() {
	ctx := kong.Parse(&CLI,
		kong.Name("Kubeflow Access Management"),
		kong.Description("User access management API server"),
		kong.DefaultEnvars(""),
	)

	ctx.Printf("elected cluster admins: %#v", CLI.ClusterAdmin)
	ctx.Printf("using user ID prefix: %#v", CLI.UserIDPrefix)
	ctx.Printf("using user ID header: %#v", CLI.UserIDHeader)

	reader, err := cache.New(ctrl.GetConfigOrDie(), cache.Options{
		Scheme: scheme.Scheme,
	})
	ctx.FatalIfErrorf(err, "could not create cached reader")
	go func() {
		ctx.FatalIfErrorf(reader.Start(ctrl.SetupSignalHandler()), "failed to start cache")
	}()
	if !reader.WaitForCacheSync(context.Background()) {
		ctx.Fatalf("failed to wait for caches to sync")
	}

	cli, err := client.New(ctrl.GetConfigOrDie(), client.Options{
		Scheme: scheme.Scheme,
	})

	cli, err = client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader: reader,
		Client:      cli,
	})
	ctx.FatalIfErrorf(err, "could not create client")

	server := apiserver.NewServer(cli, apiserver.Options{
		BaseURL:      "/kfam",
		UserIDPrefix: CLI.UserIDPrefix,
		UserIDHeader: CLI.UserIDHeader,
		Admins:       CLI.ClusterAdmin,
	})
	ctx.FatalIfErrorf(server.Run(":8081"))
}
