package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/semconv"
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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("mux-server")
var readiness = http.StatusServiceUnavailable

func getRoll(ctx context.Context, roll string) dice.RollResult {
	_, span := tracer.Start(ctx, "performRoll")
	defer span.End()

	span.AddEvent("callDiceRoll", oteltrace.WithAttributes(
		attribute.String("roll", roll)))

	res, _, _ := dice.Roll(roll)

	_, span2 := tracer.Start(ctx, "setAttributes")
	defer span2.End()
	time.Sleep(100 * time.Millisecond)
	auditStr := fmt.Sprintf("%s %s", roll, res.String())
	span2.SetAttributes(attribute.String("rollRequest", roll),
		attribute.Int("rollResult", res.Int()),
		attribute.String("rollAudit", auditStr))

	return res
}
func handler(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query()
	roll := query.Get("roll")
	if roll == "" {
		roll = "11d1" // return a 11
	}
	ctx := r.Context()
	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("rollRequest", roll))
	span.AddEvent("callGetRollFunction")

	res := getRoll(ctx, roll)

	_, span2 := tracer.Start(ctx, "writeOutput")
	defer span2.End()
	time.Sleep(100 * time.Millisecond)
	msg := fmt.Sprintf("Received request: %s\nResult: %d\n", roll, res.Int())
	_, err := w.Write([]byte(msg))
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

func initTracer() func() {
	// standard, err := stdout.NewExporter(stdout.WithPrettyPrint())
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// tp := sdktrace.NewTracerProvider(
	// 	sdktrace.WithSampler(sdktrace.AlwaysSample()),
	// 	sdktrace.WithSyncer(standard),
	// )
	// otel.SetTracerProvider(tp)
	// otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{},
	//   propagation.Baggage{}))

	// Create and install Jaeger export pipeline.
	flush, err := jaeger.InstallNewPipeline(
		jaeger.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		jaeger.WithSDKOptions(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithResource(resource.NewWithAttributes(
				semconv.ServiceNameKey.String("go-kuberoll"),
				attribute.String("exporter", "jaeger"),
			)),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	return flush
}

func main() {
	flush := initTracer()
	defer flush()
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
