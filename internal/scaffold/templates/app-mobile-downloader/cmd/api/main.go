package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "app-mobile-downloader/internal/auth/application"
	_ "app-mobile-downloader/internal/auth/http"
	_ "app-mobile-downloader/internal/auth/infrastructure/postgresql"
	_ "app-mobile-downloader/internal/auth/middleware"
	_ "app-mobile-downloader/internal/editor/http"
	_ "app-mobile-downloader/internal/home/http"
	_ "app-mobile-downloader/internal/quality/application/test_report"
	_ "app-mobile-downloader/internal/quality/http"
	_ "app-mobile-downloader/internal/scheduler/http"
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

