// Package grpcserver provides a gRPC server with graceful shutdown and health
// reporting.
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// GRPCServer is the subset of *grpc.Server used by Server.
type GRPCServer interface {
	RegisterService(desc *grpc.ServiceDesc, impl any)
	Serve(lis net.Listener) error
	GetServiceInfo() map[string]grpc.ServiceInfo
	GracefulStop()
	Stop()
}

const (
	defaultListenAddr      = ":50051"
	defaultShutdownTimeout = 30 * time.Second
)

// Server wraps a gRPC server with graceful shutdown and, optionally, a
// health service that reports SERVING while the server is running.
type Server struct {
	listenAddr      string
	shutdownTimeout time.Duration
	server          GRPCServer
	health          *health.Server
}

// New wraps the given server, typically a *grpc.Server. It listens on
// :50051 and waits up to 30s for a graceful shutdown unless configured
// otherwise. The health and reflection services are registered on the server
// if enabled. Register application services before calling New so the
// health service reports a status for them from the start.
func New(server GRPCServer, opts ...Option) *Server {
	serverOpts := &options{
		listenAddr:      defaultListenAddr,
		shutdownTimeout: defaultShutdownTimeout,
	}
	for _, opt := range opts {
		opt(serverOpts)
	}

	s := &Server{
		listenAddr:      serverOpts.listenAddr,
		shutdownTimeout: serverOpts.shutdownTimeout,
		server:          server,
	}
	if serverOpts.healthCheck {
		s.health = health.NewServer()
		healthpb.RegisterHealthServer(s.server, s.health)
	}
	if serverOpts.reflection {
		reflection.Register(s.server)
	}

	s.updateServiceHealth(healthpb.HealthCheckResponse_NOT_SERVING)
	return s
}

// updateServiceHealth sets the status of all registered services and of the
// overall server (""). It does nothing if health checks are disabled.
func (s *Server) updateServiceHealth(status healthpb.HealthCheckResponse_ServingStatus) {
	if s.health != nil {
		for service := range s.server.GetServiceInfo() {
			s.health.SetServingStatus(service, status)
		}
		s.health.SetServingStatus("", status)
	}
}

// ServeGraceful listens on the configured address and serves until ctx is
// cancelled or serving fails. Either way the server is stopped before
// returning: health is set to NOT_SERVING, open RPCs are drained, and
// remaining connections are closed once the shutdown timeout expires.
// It returns nil after a shutdown triggered by ctx, and the serve error
// otherwise. The underlying gRPC server cannot be reused afterwards.
func (s *Server) ServeGraceful(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("net listen: %w", err)
	}
	defer lis.Close()

	serveErr := make(chan error, 1)
	go func() {
		defer close(serveErr)

		slog.Info("starting grpc server", "addr", lis.Addr().String())
		defer slog.Info("grpc server shutdown complete")

		s.updateServiceHealth(healthpb.HealthCheckResponse_SERVING)
		if err := s.server.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.stop()
		return <-serveErr
	case err := <-serveErr:
		s.stop()
		return err
	}
}

// stop gracefully stops the server and forces a hard stop if draining takes
// longer than the shutdown timeout, e.g. because of long-lived streams.
func (s *Server) stop() {
	if s.health != nil {
		s.health.Shutdown()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.server.GracefulStop()
	}()

	timer := time.NewTimer(s.shutdownTimeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
		slog.Warn("graceful shutdown timed out, forcing stop", "timeout", s.shutdownTimeout)
		s.server.Stop()
		<-done
	}
}

type options struct {
	listenAddr      string
	shutdownTimeout time.Duration
	reflection      bool
	healthCheck     bool
}

// Option configures a Server.
type Option func(s *options)

// WithListenAddress sets the TCP address to listen on, e.g. ":8080". Use ":0"
// to pick a random free port.
func WithListenAddress(addr string) Option {
	return func(s *options) {
		s.listenAddr = addr
	}
}

// WithShutdownTimeout sets how long to wait for open RPCs to finish before
// forcibly closing all connections. A timeout of zero or less stops the
// server immediately. On Kubernetes, keep it below
// terminationGracePeriodSeconds.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(s *options) {
		s.shutdownTimeout = timeout
	}
}

// WithReflection registers the gRPC server reflection service, which lets
// tools like grpcurl discover the server's services.
func WithReflection() Option {
	return func(s *options) {
		s.reflection = true
	}
}

// WithHealthCheck registers the standard gRPC health service. It reports
// SERVING for the server and every registered service while ServeGraceful
// runs, and NOT_SERVING before it starts and once shutdown begins.
func WithHealthCheck() Option {
	return func(s *options) {
		s.healthCheck = true
	}
}
