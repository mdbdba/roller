package utils

import (
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
	"go.uber.org/zap"
	"os"
)

func InitTracer(logger *zap.Logger) func() {
	traceHost, envVarExists := os.LookupEnv("JAEGER_AGENT_HOST")

	if !(envVarExists) {
		traceHost = "bogus"
	}
	endPointStr := fmt.Sprintf("http://%s:14268/api/traces", traceHost)
	// Create and install Jaeger export pipeline.
	flush, err := jaeger.InstallNewPipeline(
		jaeger.WithCollectorEndpoint(endPointStr),
		jaeger.WithSDKOptions(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithResource(resource.NewWithAttributes(
				semconv.ServiceNameKey.String("roller"),
				attribute.String("exporter", "jaeger"),
			)),
		),
	)
	if err != nil {
		logger.Fatal(fmt.Sprintf("%v", err))
	}
	return flush
}
