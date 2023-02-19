package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/concourse/concourse/atc"
	"go.uber.org/zap"

	"github.com/matthope/concourse-webhook-broadcaster/internal/git"
	"github.com/matthope/concourse-webhook-broadcaster/internal/resourcecache"
	"github.com/matthope/concourse-webhook-broadcaster/internal/workqueue"
)

type WebhookHandler struct {
	Queue    *workqueue.RequestWorkqueue
	Resource *resourcecache.Cache

	Logger *zap.Logger
}

type Payload struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	CompareURL string `json:"compare"`
	Repository struct {
		FullName      string `json:"full_name"`
		CloneURL      string `json:"clone_url"`
		GitURL        string `json:"git_url"`
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
	Commits []struct {
		ID            string   `json:"id"`
		Message       string   `json:"message"`
		AddedFiles    []string `json:"added"`
		RemovedFiles  []string `json:"removed"`
		ModifiedFiles []string `json:"modified"`
	} `json:"commits"`
}

func (h *WebhookHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var pushEvent Payload

	if req.Body == nil {
		rw.WriteHeader(http.StatusBadRequest)
		h.Logger.Info("Empty body")
		fmt.Fprintf(rw, "Github: Empty body.")

		return
	}

	const mb1 = 1 << 20 // 1MB

	req.Body = http.MaxBytesReader(rw, req.Body, mb1)

	err := json.NewDecoder(req.Body).Decode(&pushEvent)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Github: failed to parse request body.")
		h.Logger.Info("failed to parse request body", zap.Error(err))

		return
	}

	if pushEvent.After == "0000000000000000000000000000000000000000" {
		h.Logger.Info("Skipping deletion event", zap.String("ref", pushEvent.Ref), zap.String("repo", pushEvent.Repository.CloneURL))

		return
	}

	if pushEvent.Repository.CloneURL == "" {
		fmt.Fprintf(rw, "Github: repository not specified in JSON body.\n")
		h.Logger.Info("repository not specified in JSON body")
	}

	h.Logger.Info("Received webhhook", zap.String("repo", pushEvent.Repository.CloneURL), zap.String("ref", pushEvent.Ref), zap.String("compare", pushEvent.CompareURL))

	found := h.process(req.Context(), pushEvent)

	if !found {
		rw.WriteHeader(http.StatusNotFound)
	}
}

func (h *WebhookHandler) process(ctx context.Context, pushEvent Payload) bool { //nolint:gocognit,cyclop,gocyclo //TODO complex
	// collect list of changed files
	filesChanged := []string{}
	for _, commit := range pushEvent.Commits {
		filesChanged = append(filesChanged, commit.AddedFiles...)
		filesChanged = append(filesChanged, commit.RemovedFiles...)
		filesChanged = append(filesChanged, commit.ModifiedFiles...)
	}

	found := false

	allowedTypes := map[string]bool{"git": true, "pull-request": true, "git-proxy": true}

	h.Resource.Scan(ctx, func(pipeline resourcecache.Pipeline, resource atc.ResourceConfig) bool {
		if _, found := allowedTypes[resource.Type]; !found {
			return true
		}

		uri, ok := resource.Source["uri"].(string)
		if !ok {
			return true
		}

		if !git.SameGitRepository(uri, pushEvent.Repository.CloneURL) {
			return true
		}

		if filterResourceGit(resource, pushEvent, h.Logger) {
			h.Logger.Info("skipping resource",
				zap.String("pipeline", pipeline.Team+"/"+pipeline.Name),
				zap.String("resource", resource.Name),
				zap.String("ref", pushEvent.Ref),
			)

			return true
		}

		if filterResourcePaths(resource.Source["paths"], filesChanged, h.Logger) {
			h.Logger.Info(
				"skipping resource due to path filter",
				zap.String("pipeline", pipeline.Team+"/"+pipeline.Name),
				zap.String("resource", resource.Name),
			)

			return true
		}

		found = true

		h.Queue.Add(resourcecache.GetWebhookURL(pipeline, resource))

		return true
	})

	return found
}

func filterResourcePaths(p any, filesChanged []string, logger *zap.Logger) bool {
	if p == nil {
		return false
	}

	resourcePaths, ok := p.([]any)
	if !ok {
		return false
	}

	if len(resourcePaths) == 0 {
		return false
	}

	paths := make([]string, 0, len(resourcePaths))

	for _, p := range resourcePaths {
		if pstring, ok := p.(string); ok {
			paths = append(paths, pstring)
		}
	}

	if len(paths) == 0 {
		return false
	}

	if !matchFiles(paths, filesChanged, logger) {
		return false
	}

	return true
}

func filterResourceGit(resource atc.ResourceConfig, pushEvent Payload, logger *zap.Logger) bool {
	allowedTypes := map[string]bool{"git": true, "git-proxy": true}
	if _, found := allowedTypes[resource.Type]; !found {
		return false
	}

	branch, _ := resource.Source["branch"].(string)
	if branch == "" {
		branch = pushEvent.Repository.DefaultBranch
	}

	ref := strings.TrimPrefix(pushEvent.Ref, "ref/heads/")

	if ref == branch {
		return false
	}

	return true
}
