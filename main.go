package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
)

func main() {
	zipServer := zip_streamer.NewServer()
	zipServer.Compression = (os.Getenv("ZS_COMPRESSION") == "DEFLATE")
	zipServer.ListfileUrlPrefix = os.Getenv("ZS_LISTFILE_URL_PREFIX")

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
		err := httpServer.ListenAndServe()

		if err != nil {
			log.Fatal(err)
		}
	}()

	// Gracefully shutdown when SIGTERM is received
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	log.Print("Received SIGTERM, shutting down...")
	httpServer.Shutdown(context.Background())
}
