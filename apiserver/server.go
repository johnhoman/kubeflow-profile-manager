package backend

import (
	"github.com/gin-gonic/gin"
	"github.com/johnhoman/kubeflow-profile-manager/backend/access"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct {
	BaseURL      string
	UserIDPrefix string
	UserIDHeader string
	Admins       []string
}


// NewServer returns a new *gin.Engine instance with the Access Management
// routes configured
func NewServer(cli client.Client, options Options) *gin.Engine {
	router := gin.Default()
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	opts := make([]access.ManagerOption, 0)
	if options.Admins != nil {
		opts = append(opts, access.WithAdmin(options.Admins...))
	}
	if options.UserIDPrefix != "" {
		opts = append(opts, access.WithUserIDPrefix(options.UserIDPrefix))
	}
	if options.UserIDHeader != "" {
		opts = append(opts, access.WithUserIDHeader(options.UserIDHeader))
	}

	mgr := access.NewManager(cli, opts...)

	grp := router.Group(options.BaseURL).Group("/v1")

	grp.GET("/role/clusteradmin", mgr.ListAdmins)

	grp.POST("/bindings", mgr.AddContributor)
	grp.DELETE("/bindings", mgr.RemoveContributor)

	grp.POST("/profiles", mgr.CreateProfile)
	grp.DELETE("/profiles/:profile", mgr.RemoveProfile)

	return router
}
