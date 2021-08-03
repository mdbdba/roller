package roll

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	muxMonitor "github.com/labbsr0x/mux-monitor"
	"github.com/mdbdba/roller/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type relationArg struct {
	Name     string
	Arg      string
	Priority int
}

const rollPath = "roll"

var logger *zap.Logger
var readiness = http.StatusServiceUnavailable
var relationArgs = []relationArg{
	{"primary", "roll=5d1", 1},
	{"secondary", "roll=7d1", 1},
	{"ancillary", "roll=9d1", 2},
	{"notImportant", "roll=11d1", 3},
}

func init() {
	logger, _ = zap.NewProduction()
	zap.ReplaceGlobals(logger)
}

func SetReadiness(newValue int) {
	readiness = newValue
}

func handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	rollRequest := query.Get("request")
	if rollRequest == "" {
		rollRequest = "11d1" // return a 11
	}
	ctx := r.Context()
	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("rollRequest", rollRequest))
	isTestVal, nbrOfRolls := IsTest(rollRequest)

	resultNbr := 0
	if isTestVal {
		resultNbr = nbrOfRolls
		span.AddEvent("TestValueDefault")
		logger.Info("Test Value Found", zap.String("rollAudit", rollRequest))
	} else {
		span.AddEvent("callGetRollFunction")

		res := GetRoll(ctx, logger, rollRequest)
		resultNbr = res.Int()
	}

	// _, span2 := tracer.Start(ctx, "writeOutput")
	span2 := oteltrace.SpanFromContext(ctx)
	defer span2.End()

	time.Sleep(100 * time.Millisecond)
	msg := fmt.Sprintf("[{\"request\":\"%s\",\"result\":%d,\"traceid\":%s}]", rollRequest,
		resultNbr,
		span.SpanContext().TraceID().String())
	_, err := w.Write([]byte(msg))
	if err != nil {
		return
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintf("[{\"response\": %d}]", http.StatusOK)))
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

func SetupRoutes(apiBasePath string) *mux.Router {

	// Creates mux-monitor instance
	monitor, err := muxMonitor.New("v1.0.0", muxMonitor.DefaultErrorMessageKey, muxMonitor.DefaultBuckets)
	if err != nil {
		panic(err)
	}
	// Create Server and Route Handlers
	r := mux.NewRouter()
	// Register mux-monitor middleware
	r.Use(monitor.Prometheus)
	r.Use(otelmux.Middleware("roller"))

	handleRoll := http.HandlerFunc(handler)
	r.Handle(fmt.Sprintf("%s/%s", apiBasePath, rollPath), cors.Middleware(handleRoll))
	r.HandleFunc("/health", healthHandler)
	r.HandleFunc("/readiness", readinessHandler)
	r.HandleFunc(fmt.Sprintf("%s/relations", apiBasePath), relationHandler)
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	return r
}
