package fairmutex

import "testing"

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
				sharedMaxBatchSize:    1024,
				sharedMaxQueueSize:    1024,
				exclusiveMaxBatchSize: 128,
				exclusiveMaxQueueSize: 128,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "custom read queue and batch",
			options: []Option{WithMaxReadQueueSize(100), WithMaxReadBatchSize(50)},
			expected: config{
				sharedMaxBatchSize:    50,
				sharedMaxQueueSize:    100,
				exclusiveMaxBatchSize: 128,
				exclusiveMaxQueueSize: 128,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "batch larger than queue gets capped",
			options: []Option{WithMaxReadQueueSize(100), WithMaxReadBatchSize(200)},
			expected: config{
				sharedMaxBatchSize:    100,
				sharedMaxQueueSize:    100,
				exclusiveMaxBatchSize: 128,
				exclusiveMaxQueueSize: 128,
				metricName:            "go.mutex.wait.seconds",
			},
		},
		{
			name:    "zero batch defaults to queue size",
			options: []Option{WithMaxReadQueueSize(64), WithMaxReadBatchSize(0)},
			expected: config{
				sharedMaxBatchSize:    64,
				sharedMaxQueueSize:    64,
				exclusiveMaxBatchSize: 128,
				exclusiveMaxQueueSize: 128,
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
