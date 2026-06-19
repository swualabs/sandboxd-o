package httpserver

import (
	"sandboxd-o/pkg/auth"
	"sandboxd-o/pkg/httplog"
	docs "sandboxd-o/sandboxd-let/docs"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func newRouter(s *Server) *gin.Engine {
	docs.SwaggerInfosandboxd.BasePath = "/"

	r := gin.New()
	r.Use(httplog.RecoveryLogger(s.log))
	r.Use(httplog.RequestLogger(s.log))

	r.GET("/", func(c *gin.Context) {
		c.File("assets/rest-ui.html")
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler, ginSwagger.InstanceName("sandboxd")))
	r.GET("/healthz", s.healthz)

	v1 := r.Group("/v1", auth.Middleware(s.sharedSecret))
	{
		v1.GET("/node/status", s.nodeStatus)
		v1.GET("/sandboxes", s.listSandboxes)
		v1.GET("/sandboxes/:id", s.getSandbox)
		v1.GET("/sandboxes/:id/logs", s.getSandboxLogs)
		v1.GET("/sandboxes/:id/containers/:name/logs", s.getContainerLogs)
		v1.POST("/sandboxes/statuses", s.sandboxStatuses)
		v1.POST("/sandboxes", s.createSandbox)
		v1.DELETE("/sandboxes/:id", s.deleteSandbox)
		v1.POST("/reconcile", s.reconcile)
	}

	return r
}
