package microdoze

import (
	"context"
	"time"

	"github.com/flohansen/microdoze/app"
	"github.com/flohansen/microdoze/logging"

	"golang.org/x/sync/errgroup"
)

// Stack contains
type Stack struct {
	g   *errgroup.Group
	ctx context.Context
}

// Deadline implements context.Context.
func (s *Stack) Deadline() (deadline time.Time, ok bool) {
	return s.ctx.Deadline()
}

// Done implements context.Context.
func (s *Stack) Done() <-chan struct{} {
	return s.ctx.Done()
}

// Err implements context.Context.
func (s *Stack) Err() error {
	return s.ctx.Err()
}

// Value implements context.Context.
func (s *Stack) Value(key any) any {
	return s.ctx.Value(key)
}

// Go starts the given functiuon in a new routine. If one function returns an
// error, the context is being canceled.
func (s *Stack) Go(f func() error) {
	s.g.Go(f)
}

// Wait blocks until all routines returned.
func (s *Stack) Wait() error {
	return s.g.Wait()
}

// NewStack returns a new Stack.
func NewStack(opts ...StackOption) *Stack {
	g, ctx := errgroup.WithContext(app.SignalContext())
	s := &Stack{
		g:   g,
		ctx: ctx,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// StackOption configures a Stack.
type StackOption func(*Stack)

// WithLogger adds the given logger to a Stack.
func WithLogger(logger logging.Logger) StackOption {
	return func(s *Stack) {
		s.ctx = logging.WithContext(s.ctx, logger)
	}
}
