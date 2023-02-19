package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/matthope/concourse-webhook-broadcaster/internal/concourse"
	"github.com/matthope/concourse-webhook-broadcaster/internal/handlers/bitbucket"
	debugh "github.com/matthope/concourse-webhook-broadcaster/internal/handlers/debug"
	"github.com/matthope/concourse-webhook-broadcaster/internal/handlers/github"
	"github.com/matthope/concourse-webhook-broadcaster/internal/handlers/gitlab"
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourcecache"
	"github.com/matthope/concourse-webhook-broadcaster/internal/workqueue"
)

func Run(ctx context.Context, params *Params, logger *zap.Logger) error { //nolint:cyclop,gocyclo // TODO complex
	ctx, shutdown := context.WithCancel(ctx)
	defer shutdown()

	client, err := concourse.NewClients(params.ConcourseURL)
	if err != nil {
		return fmt.Errorf("concourse: %w", err)
	}

	resources := resourcecache.New(logger)

	var group run.Group

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM) // Push signals into channel

	defer func() {
		signal.Stop(sigs)
		shutdown()
	}()

	// setup signal Handler
	cancelSignal := make(chan struct{})

	group.Add(func() error {
		select {
		case sig := <-sigs:
			logger.Warn("received signal, shutting down", zap.String("signal", sig.String()))
			shutdown()
		case <-cancelSignal:
		}

		return nil
	}, func(_ error) {
		close(cancelSignal)
	})

	// setup resource cache
	cancelCache := make(chan struct{})

	group.Add(func() error {
		logger.Info("starting resource cache")
		defer logger.Info("ending resource cache")

		defer shutdown()

		tick := time.NewTicker(params.RefreshInterval)
		defer tick.Stop()

		for {
			if err := resources.Update(ctx, client); err != nil {
				logger.Warn("Failed to update cache", zap.Error(err))
			}

			select {
			case <-tick.C:
			case <-cancelCache:
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	}, func(_ error) {
		close(cancelCache)
	})

	// setup workqueue
	requestQueue, err := workqueue.NewRequestWorkqueue(params.WebhookConcurrency, params.Debug, logger)
	if err != nil {
		return fmt.Errorf("error setting up request workqueue: %w", err)
	}

	cancelQueue := make(chan struct{})

	group.Add(func() error {
		logger.Info("starting request workqueue")
		defer logger.Info("ending request workqueue")

		requestQueue.Run(cancelQueue)

		return nil
	}, func(_ error) {
		close(cancelQueue)
	})

	// setup http server
	ln, err := net.Listen("tcp", params.ExtListenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %s", params.ExtListenAddr, err) //nolint:gocritic,gocyclo // TODO complex
	}

	group.Add(func() error {
		logger.Info("starting http server")
		defer logger.Info("ending http server")

		logger.Info("listening for incoming webhooks", zap.String("listen addr", ln.Addr().String()))

		mux := http.NewServeMux()

		requestCounter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of incoming HTTP requests",
			},
			[]string{"code", "method"},
		)

		if err := prometheus.Register(requestCounter); err != nil {
			return fmt.Errorf("prometheus: %w", err)
		}

		uaswitch := useragentswitcher{
			AgentMap: map[string]http.Handler{},
			mux:      mux,
		}

		uaswitch.Register(
			"github",
			`^GitHub-Hookshot/.*`,
			promhttp.InstrumentHandlerCounter(requestCounter, &github.WebhookHandler{Queue: requestQueue, Resource: resources, Logger: logger}),
		)

		uaswitch.Register(
			"bitbucket",
			`^Bitbucket-Webhooks/.*`,
			promhttp.InstrumentHandlerCounter(requestCounter, &bitbucket.WebhookHandler{Queue: requestQueue, Cache: resources, Logger: logger}),
		)

		uaswitch.Register(
			"gitlab",
			`^Gitlab/.*`,
			promhttp.InstrumentHandlerCounter(requestCounter, &gitlab.WebhookHandler{Queue: requestQueue, Resource: resources, Logger: logger}),
		)

		mux.Handle("/debug", &debugh.WebhookHandler{Queue: requestQueue, Resource: resources, Logger: logger})

		mux.Handle("/debug/shutdown", &debugh.ShutdownHandler{Shutdown: shutdown})

		mux.Handle("/health", &health{})
		mux.Handle("/healthz", &health{})

		mux.Handle("/", promhttp.InstrumentHandlerCounter(requestCounter, uaswitch))

		// TODO: check metrics, seems all is zero.
		mux.Handle("/metrics", promhttp.Handler())

		return http.Serve(ln, mux) //nolint:gosec,wrapcheck //TODO wrapcheck
	}, func(_ error) {
		ln.Close()
	})

	if err := group.Run(); err != nil {
		return fmt.Errorf("group: %w", err)
	}

	return nil
}

type useragentswitcher struct {
	AgentMap map[string]http.Handler
	mux      *http.ServeMux
}

func (u useragentswitcher) Register(name, userAgent string, handler http.Handler) {
	u.AgentMap[userAgent] = handler

	u.mux.Handle("/"+name, handler)
}

func (u useragentswitcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for ua, next := range u.AgentMap {
		if match, _ := regexp.MatchString(ua, r.UserAgent()); match {
			next.ServeHTTP(w, r)

			return
		}
	}

	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "ua match fail\nAgents: %+#v\nSupplied: %q\n", u.AgentMap, r.UserAgent())

	return
}

type health struct{}

func (h health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Healthcheck: OK\n")
}

func NewLogger(debug bool) *zap.Logger {
	logCfg := zap.NewProductionConfig()

	logCfg.DisableStacktrace = true
	logCfg.Encoding = "json"
	logCfg.EncoderConfig.StacktraceKey = ""
	logCfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	logCfg.EncoderConfig.LevelKey = "eventLevel"
	logCfg.EncoderConfig.TimeKey = "eventTimestamp"

	logCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

	if debug {
		logCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logger, err := logCfg.Build()
	if err != nil {
		panic(err)
	}

	return logger
}
