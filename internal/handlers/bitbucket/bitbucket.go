package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/concourse/concourse/atc"
	"go.uber.org/zap"

	"github.com/matthope/concourse-webhook-broadcaster/internal/git"
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourcecache"
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourceparser"
	"github.com/matthope/concourse-webhook-broadcaster/internal/workqueue"
)

type WebhookHandler struct {
	Queue *workqueue.RequestWorkqueue
	Cache *resourcecache.Cache

	Logger *zap.Logger
}

func (h *WebhookHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) { //nolint:gocyclo,cyclop // TODO complex
	var pushEvent Payload

	if req.Body == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Bitbucket: empty body\n")
		h.Logger.Info("empty body")

		return
	}

	const mb1 = 1 << 20 // 1MB

	req.Body = http.MaxBytesReader(rw, req.Body, mb1)

	err := json.NewDecoder(req.Body).Decode(&pushEvent)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Bitbucket: Body Parse Error: %s\n", err)
		h.Logger.Info("failed to parse request body", zap.Error(err))

		return
	}

	Process(req.Context(), pushEvent, h.Cache, h.Queue, rw, h.Logger)
}

type Cache interface {
	Scan(ctx context.Context, walkFn func(pipeline resourcecache.Pipeline, resource atc.ResourceConfig) bool)
}

type Queue interface {
	Add(any)
}

// func Process(ctx context.Context, pushEvent Payload, cache *resourcecache.Cache, queue *workqueue.RequestWorkqueue, rw http.ResponseWriter, logger *zap.Logger) {
func Process(ctx context.Context, pushEvent Payload, cache Cache, queue Queue, rw http.ResponseWriter, logger *zap.Logger) {
	if pushEvent.Repository == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Bitbucket: repository not specified in JSON body.\n")
		logger.Info("repository not specified in JSON body")

		return
	}

	repoBranch := pushEvent.GetBranch()

	repoURL := pushEvent.GetRepo()

	logger.Info("received webhook", zap.String("repo", repoURL))

	counter := 0

	cache.Scan(ctx, func(pipeline resourcecache.Pipeline, resource atc.ResourceConfig) bool {
		uri, ok := resourceparser.ConstructURIFromConfig(resource)

		if !ok {
			return true
		}

		if !git.SameGitRepository(uri, repoURL) {
			return true
		}

		if skipResourceGitBranch(resource, repoBranch) {
			logger.Debug(
				"skipping resource due to branch",
				zap.String("pipeline", pipeline.Name),
				zap.String("resource", resource.Name),
				zap.String("branch.changed", repoBranch),
			)

			return true
		}

		queue.Add(&resourcecache.Resource{Name: resource.Name, Pipeline: pipeline})

		counter++

		return true
	})

	logger.Info("resources found",
		zap.String("repo", repoURL),
		zap.Int("count", counter),
	)

	if counter == 0 {
		rw.WriteHeader(http.StatusNotFound)

		return
	}

	rw.WriteHeader(http.StatusAccepted)
}

func skipResourceGitBranch(resource atc.ResourceConfig, repoBranch string) bool {
	allowedTypes := map[string]bool{"git": true}
	if _, found := allowedTypes[resource.Type]; !found {
		return false
	}

	configBranch, _ := resource.Source["branch"].(string)
	if configBranch == "" {
		return false // unspecified in config
	}

	if repoBranch == "" {
		return false
	}

	if repoBranch == configBranch {
		return false // matches
	}

	return true // skip.
}
