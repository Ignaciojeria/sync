package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "app-mobile-downloader/internal/app/http"
	_ "app-mobile-downloader/internal/dev/application/test_report"
	_ "app-mobile-downloader/internal/dev/http"
	_ "app-mobile-downloader/internal/editor/http"
	_ "app-mobile-downloader/internal/shared/jwks"
	_ "app-mobile-downloader/internal/shared/server"
	_ "app-mobile-downloader/internal/shared/infrastructure/postgresql"

	"github.com/Ignaciojeria/ioc"
)

func main() {
	if err := ioc.LoadDependencies(); err != nil {
		log.Fatal(err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	if err := ioc.Shutdown(); err != nil {
		log.Fatalf("Shutdown errors: %v", err)
	}
}

