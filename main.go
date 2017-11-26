package main

import (
	"context"
	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	zipServer := zip_streamer.NewServer()

	port := os.Getenv("PORT")
	if port == "" {
		port = "4008"
	}

	httpServer := &http.Server{
		Addr:        ":" + port,
		Handler:     zipServer,
		ReadTimeout: 10 * time.Second,
	}

	log.Printf("Server starting on port %s", port)
	go func() {
		httpServer.ListenAndServe()
	}()

	// Gracefully shutdown when SIGTERM is received
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	log.Print("Received SIGTERM, shutting down...")
	httpServer.Shutdown(context.Background())
}
