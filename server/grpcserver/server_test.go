package grpcserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// fakeServer is a grpcServer whose Serve blocks until GracefulStop or Stop is
// called, or returns serveErr immediately if set.
type fakeServer struct {
	mu                sync.Mutex
	services          map[string]grpc.ServiceInfo
	serveErr          error
	blockGracefulStop bool
	onGracefulStop    func()
	gracefulStopCalls int
	stopCalls         int

	serving  chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		services: make(map[string]grpc.ServiceInfo),
		serving:  make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func (f *fakeServer) RegisterService(desc *grpc.ServiceDesc, impl any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.services[desc.ServiceName] = grpc.ServiceInfo{}
}

func (f *fakeServer) GetServiceInfo() map[string]grpc.ServiceInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	services := make(map[string]grpc.ServiceInfo, len(f.services))
	for name, info := range f.services {
		services[name] = info
	}
	return services
}

func (f *fakeServer) Serve(lis net.Listener) error {
	close(f.serving)
	if f.serveErr != nil {
		return f.serveErr
	}
	<-f.stopped
	return nil
}

func (f *fakeServer) GracefulStop() {
	f.mu.Lock()
	f.gracefulStopCalls++
	hook, block := f.onGracefulStop, f.blockGracefulStop
	f.mu.Unlock()

	if hook != nil {
		hook()
	}
	if block {
		<-f.stopped
		return
	}
	f.closeStopped()
}

func (f *fakeServer) Stop() {
	f.mu.Lock()
	f.stopCalls++
	f.mu.Unlock()
	f.closeStopped()
}

func (f *fakeServer) closeStopped() {
	f.stopOnce.Do(func() { close(f.stopped) })
}

func (f *fakeServer) calls() (gracefulStop, stop int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gracefulStopCalls, f.stopCalls
}

func serve(ctx context.Context, s *Server) <-chan error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.ServeGraceful(ctx) }()
	return errCh
}

func waitFor[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting")
		var zero T
		return zero
	}
}

func healthStatus(t *testing.T, h *health.Server, service string) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := h.Check(context.Background(), &healthpb.HealthCheckRequest{Service: service})
	require.NoError(t, err)
	return resp.Status
}

func TestNew(t *testing.T) {
	tests := []struct {
		name             string
		opts             []Option
		wantListenAddr   string
		wantTimeout      time.Duration
		wantHealth       bool
		wantRegistered   []string
		wantUnregistered []string
	}{
		{
			name:             "defaults",
			wantListenAddr:   defaultListenAddr,
			wantTimeout:      defaultShutdownTimeout,
			wantUnregistered: []string{healthpb.Health_ServiceDesc.ServiceName, "grpc.reflection.v1.ServerReflection"},
		},
		{
			name: "custom listen address and shutdown timeout",
			opts: []Option{
				WithListenAddress("127.0.0.1:1234"),
				WithShutdownTimeout(5 * time.Second),
			},
			wantListenAddr: "127.0.0.1:1234",
			wantTimeout:    5 * time.Second,
		},
		{
			name:           "health check",
			opts:           []Option{WithHealthCheck()},
			wantListenAddr: defaultListenAddr,
			wantTimeout:    defaultShutdownTimeout,
			wantHealth:     true,
			wantRegistered: []string{healthpb.Health_ServiceDesc.ServiceName},
		},
		{
			name:           "reflection",
			opts:           []Option{WithReflection()},
			wantListenAddr: defaultListenAddr,
			wantTimeout:    defaultShutdownTimeout,
			wantRegistered: []string{"grpc.reflection.v1.ServerReflection"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeServer()

			s := New(fake, tt.opts...)

			assert.Equal(t, tt.wantListenAddr, s.listenAddr)
			assert.Equal(t, tt.wantTimeout, s.shutdownTimeout)
			assert.Equal(t, tt.wantHealth, s.health != nil)
			services := fake.GetServiceInfo()
			for _, name := range tt.wantRegistered {
				assert.Contains(t, services, name)
			}
			for _, name := range tt.wantUnregistered {
				assert.NotContains(t, services, name)
			}
		})
	}
}

func TestNew_InitialHealthNotServing(t *testing.T) {
	fake := newFakeServer()

	s := New(fake, WithHealthCheck())

	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, healthStatus(t, s.health, ""))
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, healthStatus(t, s.health, healthpb.Health_ServiceDesc.ServiceName))
}

func TestServeGraceful_GracefulShutdown(t *testing.T) {
	fake := newFakeServer()
	s := New(fake, WithListenAddress("127.0.0.1:0"), WithHealthCheck())

	var statusAtGracefulStop healthpb.HealthCheckResponse_ServingStatus
	fake.onGracefulStop = func() { statusAtGracefulStop = healthStatus(t, s.health, "") }

	ctx, cancel := context.WithCancel(context.Background())
	errCh := serve(ctx, s)
	waitFor(t, fake.serving)

	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, healthStatus(t, s.health, ""))
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, healthStatus(t, s.health, healthpb.Health_ServiceDesc.ServiceName))

	cancel()

	require.NoError(t, waitFor(t, errCh))
	gracefulStops, stops := fake.calls()
	assert.Equal(t, 1, gracefulStops)
	assert.Equal(t, 0, stops)
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, statusAtGracefulStop, "health must be NOT_SERVING before draining")
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, healthStatus(t, s.health, ""))
}

func TestServeGraceful_ForcesStopAfterTimeout(t *testing.T) {
	fake := newFakeServer()
	fake.blockGracefulStop = true
	s := New(fake, WithListenAddress("127.0.0.1:0"), WithShutdownTimeout(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := serve(ctx, s)
	waitFor(t, fake.serving)

	cancel()

	require.NoError(t, waitFor(t, errCh))
	gracefulStops, stops := fake.calls()
	assert.Equal(t, 1, gracefulStops)
	assert.Equal(t, 1, stops)
}

func TestServeGraceful_ServeError(t *testing.T) {
	fake := newFakeServer()
	fake.serveErr = errors.New("accept failed")
	s := New(fake, WithListenAddress("127.0.0.1:0"), WithHealthCheck())

	err := waitFor(t, serve(context.Background(), s))

	assert.ErrorIs(t, err, fake.serveErr)
	gracefulStops, _ := fake.calls()
	assert.Equal(t, 1, gracefulStops)
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, healthStatus(t, s.health, ""))
}

func TestServeGraceful_ServerStoppedIsNotAnError(t *testing.T) {
	fake := newFakeServer()
	fake.serveErr = grpc.ErrServerStopped
	s := New(fake, WithListenAddress("127.0.0.1:0"))

	err := waitFor(t, serve(context.Background(), s))

	assert.NoError(t, err)
}

func TestServeGraceful_ListenError(t *testing.T) {
	fake := newFakeServer()
	s := New(fake, WithListenAddress("invalid-address"))

	err := s.ServeGraceful(context.Background())

	assert.ErrorContains(t, err, "net listen")
	select {
	case <-fake.serving:
		t.Fatal("Serve must not be called when listening fails")
	default:
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())
	return addr
}

func TestServeGraceful_RealServerWithOpenWatchStream(t *testing.T) {
	addr := freeAddr(t)
	s := New(grpc.NewServer(),
		WithListenAddress(addr),
		WithHealthCheck(),
		WithShutdownTimeout(100*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := serve(ctx, s)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	client := healthpb.NewHealthClient(conn)

	require.Eventually(t, func() bool {
		resp, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
		return err == nil && resp.Status == healthpb.HealthCheckResponse_SERVING
	}, 2*time.Second, 10*time.Millisecond)

	stream, err := client.Watch(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	resp, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)

	cancel()

	resp, err = stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.Status)
	assert.NoError(t, waitFor(t, errCh), "open Watch stream must not block shutdown past the timeout")
}
