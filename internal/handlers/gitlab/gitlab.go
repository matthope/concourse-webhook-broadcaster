package gitlab

import (
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
	Queue    *workqueue.RequestWorkqueue
	Resource *resourcecache.Cache

	Logger *zap.Logger
}

type Payload struct {
	Repository *Repository `json:"repository"`
}

type Repository struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	GitHTTPURL  string `json:"git_http_url"`
	GitSSHURL   string `json:"git_ssh_url"`
}

func (h *WebhookHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var pushEvent Payload

	if req.Body == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Gitlab: Empty body.")
		h.Logger.Info("empty body")

		return
	}

	const mb1 = 1 << 20 // 1MB

	req.Body = http.MaxBytesReader(rw, req.Body, mb1)

	err := json.NewDecoder(req.Body).Decode(&pushEvent)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Gitlab: failed to parse request body.")
		h.Logger.Info("failed to parse request body", zap.Error(err))

		return
	}

	if pushEvent.Repository == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Gitlab: repository not specified in JSON body.\n")
		h.Logger.Info("repository not specified in JSON body")

		return
	}

	repoURL := pushEvent.Repository.GitHTTPURL

	counter := 0

	h.Resource.Scan(req.Context(), func(pipeline resourcecache.Pipeline, resource atc.ResourceConfig) bool {
		uri, ok := resourceparser.ConstructURIFromConfig(resource)

		if !ok {
			return true
		}

		if !git.SameGitRepository(uri, repoURL) {
			return true
		}

		h.Queue.Add(resourcecache.GetWebhookURL(pipeline, resource))

		counter++

		return true
	})

	h.Logger.Info("gitlab: received webhhook", zap.String("repo", repoURL), zap.Int("resource count", counter))

	if counter == 0 {
		rw.WriteHeader(http.StatusNotFound)
	}
}
