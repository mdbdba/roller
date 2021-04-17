package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	muxMonitor "github.com/labbsr0x/mux-monitor"
	"github.com/mdbdba/dice"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
	oteltrace "go.opentelemetry.io/otel/trace"
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
)

type FakeDependencyChecker struct{}
type relationArg struct {
	Name     string
	Arg      string
	Priority int
}

var logger *zap.Logger
var tracer = otel.Tracer("mux-server")
var readiness = http.StatusServiceUnavailable
var relationArgs []relationArg

func init() {
	logger, _ = zap.NewProduction()
	zap.ReplaceGlobals(logger)

	relationArgs = []relationArg{
		{"primary", "roll=5d1", 1},
		{"secondary", "roll=7d1", 1},
		{"ancillary", "roll=9d1", 2},
		{"notImportant", "roll=11d1", 3},
	}
}

func (m *FakeDependencyChecker) GetDependencyName() string {
	return "fake-dependency"
}

func (m *FakeDependencyChecker) Check() muxMonitor.DependencyStatus {
	return muxMonitor.DOWN
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
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintf("OK: %d", http.StatusOK)))
	if err != nil {
		return
	}
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(readiness)
}

func relationHandler(w http.ResponseWriter, r *http.Request) {
	relationJson, _ := json.Marshal(relationArgs)
	_, err := w.Write([]byte(fmt.Sprintf("%s\n", relationJson)))
	if err != nil {
		return
	}
}

func initTracer(logger *zap.Logger) func() {
	traceHost, envVarExists := os.LookupEnv("JAEGER_AGENT_HOST")

	endPointStr := "http://bogus:14268/api/traces"
	if envVarExists {
		endPointStr = fmt.Sprintf("http://%s:14268/api/traces", traceHost)
	}
	// Create and install Jaeger export pipeline.
	flush, err := jaeger.InstallNewPipeline(
		jaeger.WithCollectorEndpoint(endPointStr),
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

	// Creates mux-monitor instance
	monitor, err := muxMonitor.New("v1.0.0", muxMonitor.DefaultErrorMessageKey, muxMonitor.DefaultBuckets)
	if err != nil {
		panic(err)
	}
	// Create Server and Route Handlers
	r := mux.NewRouter()
	// Register mux-monitor middleware
	r.Use(monitor.Prometheus)
	r.Use(otelmux.Middleware("go-kuberoll"))

	r.HandleFunc("/", handler)
	r.HandleFunc("/health", healthHandler)
	r.HandleFunc("/readiness", readinessHandler)
	r.HandleFunc("/relation", relationHandler)
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

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
