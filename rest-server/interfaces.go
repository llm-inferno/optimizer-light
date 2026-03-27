package rest

// interface to a REST server
type RESTServer interface {
	Run(host, port string)
}
