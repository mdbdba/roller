package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/mdbdba/dice"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("mux-server")
var readiness = http.StatusServiceUnavailable

func handler(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query()
	roll := query.Get("roll")
	if roll == "" {
		roll = "11d1" // return a 11
	}
	_, span := tracer.Start(r.Context(), "rollHandler",
		oteltrace.WithAttributes(attribute.String("request", roll)))
	defer span.End()

	// log.Printf("Received request for %s\n", roll)
	_, err := w.Write([]byte(fmt.Sprintf("Received request for %s\n", roll)))
	if err != nil {
		return
	}
	res, _, _ := dice.Roll(roll)

	span.SetAttributes(attribute.String("request", roll),
		attribute.Int("result", res.Int()),
		attribute.String("audit", res.String()))

	_, err = w.Write([]byte(fmt.Sprintf("Roll result:  %d\n", res.Int())))
	if err != nil {
		return
	}

}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintf("OK: %d", http.StatusOK)))
	if err != nil {
		return
	}
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(readiness)
}

func initTracer() {
	exporter, err := stdout.NewExporter(stdout.WithPrettyPrint())
	if err != nil {
		log.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
}

func main() {
	initTracer()
	rand.Seed(time.Now().UnixNano())

	// Create Server and Route Handlers
	r := mux.NewRouter()
	r.Use(otelmux.Middleware("go-kuberoll"))

	r.HandleFunc("/", handler)
	r.HandleFunc("/health", healthHandler)
	r.HandleFunc("/readiness", readinessHandler)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		log.Println("Starting Server")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// wait a bit before returning a positive readiness check.
	time.Sleep(15 * time.Second)
	readiness = http.StatusOK
	// Graceful Shutdown
	waitForShutdown(srv)
}

func waitForShutdown(srv *http.Server) {
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

	log.Println("Shutting down")
	os.Exit(0)
}
