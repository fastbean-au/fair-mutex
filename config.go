package fairmutex

import "go.opentelemetry.io/otel/attribute"

type config struct {
	sharedMaxBatchSize    int
	sharedMaxQueueSize    int
	exclusiveMaxBatchSize int
	exclusiveMaxQueueSize int
	metricAttributes      []attribute.KeyValue
	metricName            string
}

func getConfig(options ...Option) *config {
	// Create config with defaults
	cfg := &config{
		sharedMaxQueueSize:    1024,
		exclusiveMaxBatchSize: 32,
		exclusiveMaxQueueSize: 256,
		metricName:            "go.mutex.wait.seconds",
	}

	for _, option := range options {
		option(cfg)
	}

	// Override SharedMaxBatchSize if it is greater than the SharedMaxQueueSize
	// or is zero.
	if cfg.sharedMaxBatchSize > cfg.sharedMaxQueueSize {
		cfg.sharedMaxBatchSize = cfg.sharedMaxQueueSize
	} else if cfg.sharedMaxBatchSize == 0 {
		cfg.sharedMaxBatchSize = cfg.sharedMaxQueueSize
	}

	// Override ExclusiveMaxBatchSize if it is greater than the
	// ExclusiveMaxQueueSize.
	if cfg.exclusiveMaxBatchSize > cfg.exclusiveMaxQueueSize {
		cfg.exclusiveMaxBatchSize = cfg.exclusiveMaxQueueSize
	}

	return cfg
}

type Option func(*config)

// WithMaxReadBatchSize - the maximum batch size for read (also known as
// shared) locks. The batch size does not determine the number of calls to
// obtain a lock that are waiting, but the maximum number that will be processed
// in one locking cycle.
//
// This value cannot be larger than the MaxReadQueueSize.
//
// Defaults to the value of MaxReadQueueSize.
func WithMaxReadBatchSize(length int) Option {
	return func(c *config) {
		c.sharedMaxBatchSize = length
	}
}

// WithMaxReadQueueSize - the maximum queue size for read (also known as
// shared) locks. The queue size does not determine the number of calls to
// obtain a lock that are waiting, but the number during which we can guarantee
// order. This setting will effect the memory required.
//
// Set to 1 if this mutex will only be used as a write-only mutex.
//
// Defaults to 1024.
func WithMaxReadQueueSize(length int) Option {
	return func(c *config) {
		c.sharedMaxQueueSize = length
	}
}

// WithMaxWriteBatchSize - the maximum batch size for write (also known as
// exclusive) locks. The batch size does not determine the number of calls to
// obtain a lock that are waiting, but the maximum number that will be processed
// in one locking cycle.
//
// This value cannot be larger than the MaxWriteQueueSize.
//
// Defaults to 256.
func WithMaxWriteBatchSize(length int) Option {
	return func(c *config) {
		c.exclusiveMaxBatchSize = length
	}
}

// WithMaxWriteQueueSize - the maximum queue size for write (also known as
// exclusive) locks. The queue size does not determine the number of calls to
// obtain a lock that are waiting, but the number during which we can guarantee
// order. This setting will effect the memory required.
//
// Defaults to 32.
func WithMaxWriteQueueSize(length int) Option {
	return func(c *config) {
		c.exclusiveMaxQueueSize = length
	}
}

// WithMetricAttributes - a set of attributes with pre-set values to provide on
// every recording of the mutex lock wait time metric.
func WithMetricAttributes(attributes ...attribute.KeyValue) Option {
	return func(c *config) {
		c.metricAttributes = attributes
	}
}

// WithMetricName - name for the metric.
//
// Defaults to "go.mutex.wait.seconds".
func WithMetricName(name string) Option {
	return func(c *config) {
		c.metricName = name
	}
}
