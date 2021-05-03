package main

import (
	"context"
	"fmt"
	muxMonitor "github.com/labbsr0x/mux-monitor"
	"github.com/mdbdba/roller/roll"
	"github.com/mdbdba/roller/utils"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type FakeDependencyChecker struct{}

var logger *zap.Logger
var tracer = otel.Tracer("mux-server")

const apiBasePath = "/api"

func init() {
	logger, _ = zap.NewProduction()
	zap.ReplaceGlobals(logger)

}

func (m *FakeDependencyChecker) GetDependencyName() string {
	return "fake-dependency"
}

func (m *FakeDependencyChecker) Check() muxMonitor.DependencyStatus {
	return muxMonitor.DOWN
}

func main() {

	flush := utils.InitTracer(logger)
	defer flush()
	rand.Seed(time.Now().UnixNano())

	r := roll.SetupRoutes(apiBasePath)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		logger.Info("Starting Server")
		//log.Println("Starting Server")
		if err := srv.ListenAndServe(); err != nil {
			logger.Fatal(fmt.Sprintf("%v", err))
		}
	}()

	// wait a bit before returning a positive readiness check.
	time.Sleep(15 * time.Second)
	roll.SetReadiness(http.StatusOK)
	// Graceful Shutdown
	waitForShutdown(srv, logger)
}

func waitForShutdown(srv *http.Server, logger *zap.Logger) {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	err := srv.Shutdown(ctx)
	if err != nil {
		return
	}

	logger.Info("Shutting down")
}
