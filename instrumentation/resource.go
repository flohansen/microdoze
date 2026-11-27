package instrumentation

import (
	"context"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// newResource describes this service for traces and metrics. The service name
// defaults to "unknown_service:<executable>", following the OpenTelemetry
// convention, and can be overridden through the standard OTEL_SERVICE_NAME and
// OTEL_RESOURCE_ATTRIBUTES environment variables.
func newResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceName(defaultServiceName())),
		resource.WithFromEnv(),
	)
}

func defaultServiceName() string {
	return "unknown_service:" + filepath.Base(os.Args[0])
}
