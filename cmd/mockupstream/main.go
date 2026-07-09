package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/restitch/restitch-gateway/internal/mockupstream"
)

func main() {
	port := flag.Int("port", 8081, "server port")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("mockupstream listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mockupstream.Handler()))
}
