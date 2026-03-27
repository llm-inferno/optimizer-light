package rest

import (
	"github.com/gin-gonic/gin"
	"github.com/llm-inferno/optimizer-light/pkg/core"
)

// global pointer to system
var system *core.System

// Base REST server
type BaseServer struct {
	router *gin.Engine
}

func NewBaseServer() *BaseServer {
	return &BaseServer{
		router: gin.Default(),
	}
}

// start server
func (server *BaseServer) Run(host, port string) {
	// instantiate a clean system
	system = core.NewSystem()

	_ = server.router.Run(host + ":" + port)
}
