package workqueue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"

	_ "github.com/matthope/concourse-webhook-broadcaster/internal/prometheus" // so that the init gets triggered
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourcecache"
)

const (
	WorkerBaseDelay = 5 * time.Second
	WorkerMaxDelay  = 60 * time.Second
)

type RequestWorkqueue struct {
	queue       workqueue.RateLimitingInterface
	threadiness int

	webhooksSuccess prometheus.Counter
	webhooksErrors  prometheus.Counter

	logger *zap.Logger

	debug bool
}

func NewRequestWorkqueue(threadiness int, debug bool, logger *zap.Logger) (*RequestWorkqueue, error) {
	wq := &RequestWorkqueue{
		queue:       workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(WorkerBaseDelay, WorkerMaxDelay), "webhook"),
		threadiness: threadiness,
		webhooksSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "webhook",
			Name:      "success_total",
			Help:      "Total number of successfully delivered webhooks",
		}),
		webhooksErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "webhook",
			Name:      "errors_total",
			Help:      "Total number of successfully delivered webhooks",
		}),
		logger: logger,
		debug:  debug,
	}

	if err := prometheus.Register(wq.webhooksSuccess); err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}

	if err := prometheus.Register(wq.webhooksErrors); err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}

	return wq, nil
}

func (c *RequestWorkqueue) Add(url any) {
	c.queue.Add(url)
}

func (c *RequestWorkqueue) Run(stopCh <-chan struct{}) {
	defer c.queue.ShutDown()

	for i := 0; i < c.threadiness; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *RequestWorkqueue) worker() {
	ctx := context.Background()

	for c.processNextWorkItem(ctx) {
	}
}

func (c *RequestWorkqueue) processNextWorkItem(ctx context.Context) bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	defer c.queue.Done(key)

	var err error

	switch k := key.(type) {
	case string:
		err = c.performS(ctx, k)
	case *resourcecache.Resource:
		err = c.performR(ctx, k)
	}

	if err != nil {
		c.webhooksErrors.Inc()
	} else {
		c.webhooksSuccess.Inc()
	}

	if err != nil {
		c.logger.Error("error sending webhook", zap.Error(err))
	}

	if err != nil && c.queue.NumRequeues(key) < 5 {
		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)

		return true
	}

	// clear the rate limit history on successful processing
	c.queue.Forget(key)

	return true
}

var ErrRequestFailed = errors.New("request failed")

func (c *RequestWorkqueue) performS(ctx context.Context, url string) error {
	tokenRegexp := regexp.MustCompile(`webhook_token=[^&]+`)

	redactedURL := tokenRegexp.ReplaceAllString(url, "webhook_token=[REDACTED]")

	if c.debug {
		c.logger.Info("DRY RUN: Calling POST", zap.String("url", redactedURL))

		return nil
	}

	c.logger.Info("Calling POST", zap.String("url", redactedURL))

	// TODO: should we really retry if concourse returns a 401? 404? etc..

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRequestFailed, err)
	}

	req.Header.Add("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(req)
	if err != nil || response.StatusCode >= 400 {
		return fmt.Errorf("%w: %q, response: %q %w", ErrRequestFailed, redactedURL, response.Status, err)
	}

	response.Body.Close()

	return nil
}

func (c *RequestWorkqueue) performR(_ context.Context, r *resourcecache.Resource) error {
	logger := c.logger.With(zap.String("pipeline", r.Pipeline.String()), zap.String("resource", r.Name))

	logger.Info("refreshing resource via api")

	cclient, err := r.Pipeline.Concourse.RefreshClientWithToken()
	if err != nil {
		return fmt.Errorf("concourse auth: %w", err)
	}

	_, found, err := cclient.Team(r.Pipeline.Team).CheckResource(r.Pipeline.Ref(), r.Name, nil, false)

	if !found {
		logger.Info("resource not found")

		return nil // dont retry
	}

	if err != nil {
		return fmt.Errorf("concourse: %w", err)
	}

	logger.Debug("refreshed successfully")

	return nil
}
