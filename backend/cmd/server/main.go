package main

import (
	"log"
	"net/http"

	"github.com/aschi2/MultiplayerBillSplit/backend/internal/server"
)

func main() {
	config := server.LoadConfig()
	srv, err := server.NewServer(config)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("starting backend on :%s", config.Port)
	if err := http.ListenAndServe(":"+config.Port, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}
