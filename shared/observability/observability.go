package observability

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// SetupTracing initializes OpenTelemetry tracing with stdout exporter (for demo; replace with OTLP in prod)
func SetupTracing(serviceName string) func() {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("failed to initialize stdouttrace exporter: %v", err)
	}
	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	provider := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	return func() { _ = provider.Shutdown(nil) }
}

// SetupPrometheusMetrics initializes Prometheus metrics exporter and exposes /metrics endpoint
func SetupPrometheusMetrics() *metric.MeterProvider {
	exp, err := prometheus.New()
	if err != nil {
		log.Fatalf("failed to initialize prometheus exporter: %v", err)
	}
	mp := metric.NewMeterProvider(metric.WithReader(exp))
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		_ = http.ListenAndServe(":2112", nil)
	}()
	return mp
}
