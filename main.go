package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/semconv"
	"go.uber.org/zap"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
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

var logger *zap.Logger
var tracer = otel.Tracer("mux-server")
var readiness = http.StatusServiceUnavailable

func init() {
	logger, _ = zap.NewProduction()
	zap.ReplaceGlobals(logger)
}

func isTest(rollDesc string) (bool, int) {
	p1 := "5|7|9|11d1$"
	res, _ := regexp.MatchString(p1, rollDesc)
	rolls := 0
	if res {
		rolls, _ = strconv.Atoi(strings.Split(rollDesc, "d")[0])
	}
	return res, rolls
}

func getRoll(ctx context.Context, roll string) dice.RollResult {
	_, span := tracer.Start(ctx, "performRoll")
	span.AddEvent("callDiceRoll", oteltrace.WithAttributes(
		attribute.String("roll", roll)))

	res, _, _ := dice.Roll(roll)

	span.SetAttributes(attribute.Int("rollResult", res.Int()))
	span.End()

	_, span2 := tracer.Start(ctx, "setAttributes")
	defer span2.End()
	time.Sleep(100 * time.Millisecond)
	auditStr := fmt.Sprintf("%s %s", roll, res.String())
	span2.SetAttributes(attribute.String("rollRequest", roll),
		attribute.Int("rollResult", res.Int()),
		attribute.String("rollAudit", auditStr))
	logger.Info("getRoll performed", zap.String("rollAudit", auditStr))

	return res
}

func handler(w http.ResponseWriter, r *http.Request) {

	logger.Info("handler Triggered")
	query := r.URL.Query()
	roll := query.Get("roll")
	if roll == "" {
		roll = "11d1" // return a 11
	}
	ctx := r.Context()
	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("rollRequest", roll))
	isTestVal, nbrOfRolls := isTest(roll)

	resultNbr := 0
	if isTestVal {
		resultNbr = nbrOfRolls
		span.AddEvent("TestValueDefault")
		logger.Info("Test Value Found", zap.String("rollAudit", roll))
	} else {
		span.AddEvent("callGetRollFunction")

		res := getRoll(ctx, roll)
		resultNbr = res.Int()
	}

	_, span2 := tracer.Start(ctx, "writeOutput")
	defer span2.End()

	time.Sleep(100 * time.Millisecond)
	msg := fmt.Sprintf("Received request: %s\nResult: %d\n", roll, resultNbr)
	_, err := w.Write([]byte(msg))
	if err != nil {
		return
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	logger.Info("healthHandler Triggered")
	_, span := tracer.Start(r.Context(), "healthHandler")
	defer span.End()
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintf("OK: %d", http.StatusOK)))
	if err != nil {
		return
	}
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	logger.Info("readinessHandler Triggered")
	_, span := tracer.Start(r.Context(), "healthHandler")
	defer span.End()
	w.WriteHeader(readiness)
}

func initTracer(logger *zap.Logger) func() {

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
		logger.Fatal(fmt.Sprintf("%v", err))
	}
	return flush
}

func main() {

	flush := initTracer(logger)
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
		logger.Info("Starting Server")
		//log.Println("Starting Server")
		if err := srv.ListenAndServe(); err != nil {
			logger.Fatal(fmt.Sprintf("%v", err))
		}
	}()

	// wait a bit before returning a positive readiness check.
	time.Sleep(15 * time.Second)
	readiness = http.StatusOK
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
