/*
Copied from https://github.com/kubernetes/kubernetes/blob/master/pkg/util/workqueue/prometheus/prometheus.go
Changes: Converted summaries to histgramms

Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package prometheus

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/workqueue"
)

// Package prometheus sets the workqueue DefaultMetricsFactory to produce
// prometheus metrics. To use this package, you just have to import it.

func init() { //nolint:gochecknoinits // TODO init
	workqueue.SetProvider(prometheusMetricsProvider{})
}

type prometheusMetricsProvider struct{}

func (prometheusMetricsProvider) NewDepthMetric(name string) workqueue.GaugeMetric {
	depth := prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: name,
		Name:      "depth",
		Help:      "Current depth of workqueue: " + name,
	})

	if err := prometheus.Register(depth); err != nil {
		panic(err)
	}

	return depth
}

func (prometheusMetricsProvider) NewAddsMetric(name string) workqueue.CounterMetric {
	adds := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: name,
		Name:      "adds",
		Help:      "Total number of adds handled by workqueue: " + name,
	})

	if err := prometheus.Register(adds); err != nil {
		panic(err)
	}

	return adds
}

func (prometheusMetricsProvider) NewLatencyMetric(name string) workqueue.HistogramMetric {
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Subsystem: name,
		Name:      "queue_latency_seconds",
		Help:      "How long an item stays in workqueue" + name + " before being requested.",
		Buckets:   []float64{.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
	})

	if err := prometheus.Register(latency); err != nil {
		panic(err)
	}

	return latency
}

func (prometheusMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	workDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Subsystem: name,
		Name:      "work_duration_seconds",
		Help:      "How long in seconds processing an item from workqueue takes.",
		Buckets:   []float64{.5, 1, 2.5, 5, 10, 20, 40, 60, 120},
	})

	if err := prometheus.Register(workDuration); err != nil {
		panic(err)
	}

	return workDuration
}

func (prometheusMetricsProvider) NewRetriesMetric(name string) workqueue.CounterMetric {
	retries := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: name,
		Name:      "retries",
		Help:      "Total number of retries handled by workqueue: " + name,
	})

	if err := prometheus.Register(retries); err != nil {
		panic(err)
	}

	return retries
}

func (prometheusMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	retries := prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: name,
		Name:      "longest_running_processor_seconds",
		Help:      "How many seconds has the longest running processor for workqueue been running",
	})

	if err := prometheus.Register(retries); err != nil {
		panic(err)
	}

	return retries
}

func (prometheusMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	unfinished := prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: name,
		Name:      "unfinished_work_seconds",
		Help:      "How many seconds of work has done that is in progress and hasn't been observed by work_duration. Large values indicate stuck threads. One can deduce the number of stuck threads by observing the rate at which this increases.",
	})

	if err := prometheus.Register(unfinished); err != nil {
		panic(err)
	}

	return unfinished
}
