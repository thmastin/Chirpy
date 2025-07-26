package main

import (
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	var s http.Server
	s.Handler = mux
	s.Addr = ":8080"

	s.ListenAndServe()

}
