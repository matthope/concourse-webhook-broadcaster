package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/concourse/concourse/atc"
	"go.uber.org/zap"

	"github.com/matthope/concourse-webhook-broadcaster/internal/git"
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourcecache"
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourceparser"
	"github.com/matthope/concourse-webhook-broadcaster/internal/workqueue"
)

type WebhookHandler struct {
	Queue    *workqueue.RequestWorkqueue
	Resource *resourcecache.Cache

	Logger *zap.Logger
}

type Payload struct {
	GitURL string `json:"url"`
}

func (h *WebhookHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var pushEvent Payload

	if req.Body == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "debug: empty body\n")
		h.Logger.Info("empty body")

		return
	}

	err := json.NewDecoder(req.Body).Decode(&pushEvent)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "debug: Body Parse Error: %s\n", err)
		h.Logger.Info("failed to parse request body", zap.Error(err))

		return
	}

	filter := resourcecache.Filter{
		Pipeline: req.URL.Query().Get("pipeline"),
		Team:     req.URL.Query().Get("team"),
	}

	fmt.Fprintf(rw, "Request: %+v\n", pushEvent)

	if pushEvent.GitURL != "" {
		findRepo(req.Context(), rw, h.Resource, filter, pushEvent.GitURL)
	} else {
		outputAll(req.Context(), rw, h.Resource, filter)
	}
}

func findRepo(ctx context.Context, rw io.Writer, resources *resourcecache.Cache, filter resourcecache.Filter, repo string) {
	counter := 0

	resources.Scan(ctx, func(pipeline resourcecache.Pipeline, resource atc.ResourceConfig) bool {
		if !filter.Match(pipeline) {
			return true
		}

		counter++

		if uri, ok := resourceparser.ConstructURIFromConfig(resource); ok {
			if git.SameGitRepository(uri, repo) {
				fmt.Fprintf(rw, "Found: %s/%s%s %q [%s]\n", pipeline.Team, pipeline.Name, pipeline.InstanceVars, resource.Name, resource.Type)
			}
		}

		return true
	})

	fmt.Fprintf(rw, "# Searched %d entries\n", counter)
}

func outputAll(ctx context.Context, rw io.Writer, resources *resourcecache.Cache, filter resourcecache.Filter) {
	counter := 0

	resources.Scan(ctx, func(pipeline resourcecache.Pipeline, resource atc.ResourceConfig) bool {
		if !filter.Match(pipeline) {
			return true
		}

		counter++

		resourceURI, _ := resourceparser.ConstructURIFromConfig(resource)

		fmt.Fprintf(rw, "Found: %s/%s%s %q [%s] - %s\n", pipeline.Team, pipeline.Name, pipeline.InstanceVars, resource.Name, resource.Type, resourceURI)

		return true
	})

	fmt.Fprintf(rw, "# Searched %d entries\n", counter)
}

type ShutdownHandler struct {
	Shutdown context.CancelFunc
}

func (h *ShutdownHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if h.Shutdown != nil {
		h.Shutdown()
	}
}
