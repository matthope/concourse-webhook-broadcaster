package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/concourse/concourse/atc"
)

type GithubWebhookHandler struct {
	queue *RequestWorkqueue
}

func (gh *GithubWebhookHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	var pushEvent struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
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
	if req.Body == nil {
		rw.WriteHeader(400)
		log.Printf("Empty body")
		return
	}
	err := json.NewDecoder(req.Body).Decode(&pushEvent)
	if err != nil {
		rw.WriteHeader(400)
		log.Printf("Failed to parse request body: %s", err)
		return
	}

	if pushEvent.After == "0000000000000000000000000000000000000000" {
		log.Printf("Skipping deletion event for ref %s in %s", pushEvent.Ref, pushEvent.Repository.CloneURL)
		return
	}
	log.Printf("Received webhhook for %s, ref %s, after %s", pushEvent.Repository.CloneURL, pushEvent.Ref, pushEvent.After)

	//collect list of changed files
	filesChanged := []string{}
	for _, commit := range pushEvent.Commits {
		filesChanged = append(filesChanged, commit.AddedFiles...)
		filesChanged = append(filesChanged, commit.RemovedFiles...)
		filesChanged = append(filesChanged, commit.ModifiedFiles...)
	}

	ScanResourceCache(func(pipeline Pipeline, resource atc.ResourceConfig) bool {
		if resource.Type != "git" && resource.Type != "pull-request" && resource.Type != "git-proxy" {
			return true
		}
		if uri, ok := resource.Source["uri"].(string); ok {
			if SameGitRepository(uri, pushEvent.Repository.CloneURL) {
				if resource.Type == "git" || resource.Type == "git-proxy" {
					//skip, if push is for branch not tracked by resource
					branch, _ := resource.Source["branch"].(string)
					if branch == "" {
						branch = pushEvent.Repository.DefaultBranch
					}
					if strings.TrimPrefix(pushEvent.Ref, "refs/heads/") != branch {
						log.Printf("Skipping resource %s/%s in team %s. Which is tracking branch %s", pipeline.Name, resource.Name, pipeline.Team, branch)
						return true
					}
				}

				//skip if path filter of resource does not match any of the changed files
				if ps, ok := resource.Source["paths"].([]interface{}); ok && len(ps) > 1 {
					paths := make([]string, 0, len(ps))
					for _, p := range ps {
						if pstring, ok := p.(string); ok {
							paths = append(paths, pstring)
						}
					}
					if len(paths) > 1 && !matchFiles(paths, filesChanged) {
						log.Printf("Skipping resource %s/%s in team %s, due to path filter", pipeline.Name, resource.Name, pipeline.Team)
						return true
					}
				}
				webhookURL := fmt.Sprintf("%s/api/v1/teams/%s/pipelines/%s/resources/%s/check/webhook?webhook_token=%s",
					concourseURL,
					pipeline.Team,
					pipeline.Name,
					resource.Name,
					resource.WebhookToken,
				)
				gh.queue.Add(webhookURL)
			}
		}
		return true
	})

}

func matchFiles(patterns []string, files []string) bool {
	for _, file := range files {
		for _, pattern := range patterns {
			// direct match
			if file == pattern {
				return true
			}
			// directory match
			if strings.HasSuffix(pattern, "/") && strings.HasPrefix(file, pattern) {
				return true
			}
			// directory without trainling / match
			if strings.HasPrefix(file, pattern+"/") {
				return true
			}
			//last resort glob match
			if ok, _ := filepath.Match(pattern, file); ok {
				return true
			}
		}
	}
	return false
}
