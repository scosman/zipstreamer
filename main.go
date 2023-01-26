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

	shutdownChannel := make(chan os.Signal, 10)
	go func() {
		log.Printf("Server starting on port %s", port)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Server Error: %s", err)
		}
		shutdownChannel <- syscall.SIGUSR1
	}()

	// Listen for os signal for graceful shutdown
	signal.Notify(shutdownChannel, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	// Wait for shutdown signal, then shut down
	shutdownSignal := <-shutdownChannel
	log.Printf("Received signal (%s), shutting down...", shutdownSignal.String())
	httpServer.Shutdown(context.Background())

	// Exit was not expected, return non 0 exit code
	if shutdownSignal == syscall.SIGUSR1 {
		os.Exit(1)
	}
}
