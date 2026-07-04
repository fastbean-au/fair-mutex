package fairmutex

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestMutexConfigDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name     string
		options  []Option
		expected config
	}{
		{
			name:    "defaults",
			options: []Option{},
			expected: config{
				sharedMaxBatchSize:    256,
				sharedMaxQueueSize:    1024,
				exclusiveMaxBatchSize: 32,
				exclusiveMaxQueueSize: 256,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "custom read queue and batch",
			options: []Option{WithMaxReadQueueSize(100), WithMaxReadBatchSize(50)},
			expected: config{
				sharedMaxBatchSize:    50,
				sharedMaxQueueSize:    100,
				exclusiveMaxBatchSize: 32,
				exclusiveMaxQueueSize: 256,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "batch larger than queue gets capped",
			options: []Option{WithMaxReadQueueSize(100), WithMaxReadBatchSize(200)},
			expected: config{
				sharedMaxBatchSize:    100,
				sharedMaxQueueSize:    100,
				exclusiveMaxBatchSize: 32,
				exclusiveMaxQueueSize: 256,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "zero batch defaults to queue size",
			options: []Option{WithMaxReadQueueSize(64), WithMaxReadBatchSize(0)},
			expected: config{
				sharedMaxBatchSize:    1,
				sharedMaxQueueSize:    64,
				exclusiveMaxBatchSize: 32,
				exclusiveMaxQueueSize: 256,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "write-only config",
			options: []Option{WithMaxReadQueueSize(1), WithMaxWriteQueueSize(10)},
			expected: config{
				sharedMaxBatchSize:    1,
				sharedMaxQueueSize:    1,
				exclusiveMaxBatchSize: 10,
				exclusiveMaxQueueSize: 10,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "batch sizes of 0",
			options: []Option{WithMaxReadBatchSize(0), WithMaxWriteBatchSize(0)},
			expected: config{
				sharedMaxBatchSize:    1,
				sharedMaxQueueSize:    1024,
				exclusiveMaxBatchSize: 1,
				exclusiveMaxQueueSize: 256,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "batch sizes of -1",
			options: []Option{WithMaxReadBatchSize(-1), WithMaxWriteBatchSize(-1)},
			expected: config{
				sharedMaxBatchSize:    1,
				sharedMaxQueueSize:    1024,
				exclusiveMaxBatchSize: 1,
				exclusiveMaxQueueSize: 256,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			// Exposes the clamping-order bug: a queue size of zero was left
			// at zero, and dragged the batch size down to zero with it - the
			// batch size's own minimum-of-one clamp never ran because it was
			// in an else-if behind the larger-than-queue check.
			name:    "zero queue sizes get clamped",
			options: []Option{WithMaxReadQueueSize(0), WithMaxWriteQueueSize(0)},
			expected: config{
				sharedMaxBatchSize:    1,
				sharedMaxQueueSize:    1,
				exclusiveMaxBatchSize: 1,
				exclusiveMaxQueueSize: 1,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			// Exposes the missing validation of queue sizes: a negative size
			// panicked in make() when New created the queue channels.
			name:    "negative queue sizes get clamped",
			options: []Option{WithMaxReadQueueSize(-5), WithMaxWriteQueueSize(-5)},
			expected: config{
				sharedMaxBatchSize:    1,
				sharedMaxQueueSize:    1,
				exclusiveMaxBatchSize: 1,
				exclusiveMaxQueueSize: 1,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "metrics config",
			options: []Option{WithMetricName("my.metric"), WithMetricAttributes(attribute.Int("my_attribute", 42))},
			expected: config{
				sharedMaxBatchSize:    256,
				sharedMaxQueueSize:    1024,
				exclusiveMaxBatchSize: 32,
				exclusiveMaxQueueSize: 256,
				metricName:            "my.metric",
				metricAttributes:      []attribute.KeyValue{attribute.Int("my_attribute", 42)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := getConfig(tt.options...)
			if cfg.sharedMaxBatchSize != tt.expected.sharedMaxBatchSize ||
				cfg.sharedMaxQueueSize != tt.expected.sharedMaxQueueSize ||
				cfg.exclusiveMaxBatchSize != tt.expected.exclusiveMaxBatchSize ||
				cfg.exclusiveMaxQueueSize != tt.expected.exclusiveMaxQueueSize ||
				cfg.metricName != tt.expected.metricName {
				t.Errorf("Config mismatch. Got: %+v, Want: %+v", *cfg, tt.expected)
			}
		})
	}
}
