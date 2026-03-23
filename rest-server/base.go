package rest

import (
	"os"

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
func (server *BaseServer) Run() {
	// instantiate a clean system
	system = core.NewSystem()

	host := ""
	port := "8080"
	if h := os.Getenv(RestHostEnvName); h != "" {
		host = h
	}
	if p := os.Getenv(RestPortEnvName); p != "" {
		port = p
	}
	_ = server.router.Run(host + ":" + port)
}
