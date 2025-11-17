# Microdoze

A framework to quickly spin up microservices.

## Table of Contents

- [Logging](#logging)

## Logging

```go
func main() {
    // Create slog logger.
    log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    // Create new stack context
    stack := microdoze.NewStack(
        microdoze.WithLogging(logging.NewFromSlog(log)),
    )

    // Run the application using stack context.
    if err := run(stack); err != nil {
        // handle error
    }
}

func run(ctx context.Context) error {
    log := logging.FromContext(ctx)
    log.Info("running...")
    return nil
}
```
