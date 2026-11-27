package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const defaultServiceName = "agentoffice"

// newResource describes this service for traces and metrics. The service name
// defaults to defaultServiceName and can be overridden through the standard
// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES environment variables.
func newResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceName(defaultServiceName)),
		resource.WithFromEnv(),
	)
}
