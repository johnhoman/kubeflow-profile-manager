package access

import (
	"github.com/gin-gonic/gin"
)

type Manager interface {
	AddContributor(c *gin.Context)
	RemoveContributor(c *gin.Context)
	CreateProfile(c *gin.Context)
	RemoveProfile(c *gin.Context)
	ListAdmins(c *gin.Context)
}
