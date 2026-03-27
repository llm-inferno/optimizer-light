package main

import (
	"os"

	rest "github.com/llm-inferno/optimizer-light/rest-server"
)

// create and run a REST API Optimizer server
//   - stateless (default) or statefull (with -F argument)
func main() {
	host := os.Getenv(rest.RestHostEnvName)
	if host == "" {
		host = rest.DefaultRestHost
	}
	port := os.Getenv(rest.RestPortEnvName)
	if port == "" {
		port = rest.DefaultRestPort
	}

	var server rest.RESTServer
	statefull := len(os.Args) > 1 && os.Args[1] == rest.DefaultStatefull
	if statefull {
		server = rest.NewStateFullServer()
	} else {
		server = rest.NewStateLessServer()
	}
	server.Run(host, port)
}
