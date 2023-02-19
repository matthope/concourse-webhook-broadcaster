package resourcecache

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sync"

	"github.com/concourse/concourse/atc"
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	"go.uber.org/zap"

	"github.com/matthope/concourse-webhook-broadcaster/internal/concourse"
)

type Pipeline struct {
	ID           int
	Name         string
	Version      string
	Team         string
	InstanceVars atc.InstanceVars
	Resources    []atc.ResourceConfig
	Concourse    *concourse.Client
}

func (p Pipeline) String() string {
	if p.InstanceVars == nil {
		return fmt.Sprintf("%s/%s", p.Team, p.Name)
	}

	return fmt.Sprintf("%s/%s/%s", p.Team, p.Name, p.InstanceVars.String())
}

func (p Pipeline) Ref() atc.PipelineRef {
	return atc.PipelineRef{Name: p.Name, InstanceVars: p.InstanceVars}
}

type Resource struct {
	Pipeline Pipeline
	Name     string
}

func New(logger *zap.Logger) *Cache {
	c := &Cache{logger: logger}

	// TODO: optional filters

	return c
}

type Cache struct {
	resourceCache sync.Map
	filter        Filter
	logger        *zap.Logger
}

type Filter struct {
	ConcourseURL string
	Pipeline     string
	Team         string
}

func (f *Filter) Match(p Pipeline) bool {
	if f.Team != "" && f.Team != p.Team {
		return false
	}

	if f.Pipeline != "" && f.Pipeline != p.Name {
		return false
	}

	return true
}

func (c *Cache) Update(ctx context.Context, cclient *concourse.Clients) error { //nolint:cyclop,gocognit,gocyclo // TODO complex
	c.logger.Info("Starting cache update")

	const initialPipelineGuess = 50

	pipelinesByID := make(map[string]struct{}, initialPipelineGuess)

	for _, cc := range cclient.Clients {
		client, err := cc.RefreshClientWithToken()
		if err != nil {
			return fmt.Errorf("concourse client: %w", err) // TODO errlint
		}

		logger := c.logger.With(zap.String("ci", cc.URL()))

		if c.filter.ConcourseURL != "" && c.filter.ConcourseURL != cc.URL() {
			c.logger.Debug("Skipping due to filter", zap.String("ci", cc.URL()))

			continue
		}

		teams, err := client.ListTeams()
		if err != nil {
			return fmt.Errorf("Failed to list teams: %s", err) //nolint:errorlint,goerr113,stylecheck //TODO errlint
		}

		c.logger.Info("updating teams", zap.Int("count", len(teams)))

		for _, team := range teams {
			clientTeam := client.Team(team.Name)

			teamLogger := logger.With(zap.String("team", team.Name))

			if c.filter.Team != "" && c.filter.Team != team.Name {
				teamLogger.Debug("Skipping due to filter")

				continue
			}

			pipelines, err := clientTeam.ListPipelines()
			if err != nil {
				return fmt.Errorf("Failed to list pipelines: %s", err) //nolint:errorlint,goerr113,stylecheck //TODO errlint
			}

			teamLogger.Debug("processing pipelines", zap.Int("count", len(pipelines)))

			// update pipeline cache
			for _, pipeline := range pipelines {
				pipelineID := cc.PipelineID(pipeline)

				// temporary memorize pipelines from team to cleanup after the teams loop
				pipelinesByID[pipelineID] = struct{}{}

				if err = updateFromPipeline(
					ctx,
					pipeline,
					&c.resourceCache,
					cc,
					clientTeam,
					c.filter,
					c.logger.With(zap.String("pipeline", pipelineID)),
				); err != nil {
					return err
				}
			}
		}
	}

	deleteUnseen(&c.resourceCache, pipelinesByID, c.logger)

	c.logger.Info("ending cache update")

	return nil
}

func updateFromPipeline(ctx context.Context, pipeline atc.Pipeline, resourceCache *sync.Map, cc *concourse.Client, clientTeam goconcourse.Team, filter Filter, logger *zap.Logger) error {
	pipelineID := cc.PipelineID(pipeline)

	if filter.Pipeline != "" && filter.Pipeline != pipeline.Name {
		return nil
	}

	config, version, found, err := clientTeam.PipelineConfig(pipeline.Ref())
	if err != nil {
		logger.Error("failed to get pipeline", zap.Error(err))

		return nil
	}

	if !found {
		return nil
	}

	cachedPipeline, inCache := resourceCache.Load(pipelineID)

	// add or replace cache for pipeline
	if !inCache || cachedPipeline.(Pipeline).Version != version { //nolint:forcetypeassert // TODO type
		newCacheObj := Pipeline{
			ID:           pipeline.ID,
			Name:         pipeline.Name,
			Team:         pipeline.TeamName,
			InstanceVars: pipeline.InstanceVars,
			Version:      version,
			Concourse:    cc,
		}

		for _, resource := range config.Resources {
			if resource.WebhookToken == "" {
				// Skip resources without webhook tokens
				continue
			}

			newCacheObj.Resources = append(newCacheObj.Resources, resource)
		}

		resourceCache.Store(pipelineID, newCacheObj)

		// logger.Info("new version detected", zap.Int("found_resources", len(newCacheObj.Resources)))

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

func (c *Cache) Store(ctx context.Context, key string, val atc.ResourceConfig) {
	c.resourceCache.Store(key, val)
}

func (c *Cache) List(ctx context.Context, w io.Writer) {
	c.resourceCache.Range(func(key, value any) bool {
		fmt.Fprintf(w, "- %s\n", key)

		return true
	})
}

func (c *Cache) Healthy(ctx context.Context) bool {
	return true
}

// deleteUnseen will delete removed pipelines from cache.
func deleteUnseen(m *sync.Map, seen map[string]struct{}, logger *zap.Logger) {
	m.Range(func(key, _ any) bool {
		pipelineID, ok := key.(string)
		if !ok {
			return true // skip
		}

		if _, found := seen[pipelineID]; !found {
			logger.Info("removing vanished pipeline", zap.String("pipeline", pipelineID))

			m.Delete(pipelineID)
		}

		return true
	})
}

func (c *Cache) Scan(ctx context.Context, walkFn func(pipeline Pipeline, resource atc.ResourceConfig) bool) {
	c.resourceCache.Range(func(_, val any) bool {
		pipeline, ok := val.(Pipeline)
		if !ok {
			return true
		}

		for _, resource := range pipeline.Resources {
			if !walkFn(pipeline, resource) {
				return false
			}
		}

		return true
	})
}

func GetWebhookURL(pipeline Pipeline, resource atc.ResourceConfig) string {
	u, _ := url.Parse(pipeline.Concourse.URL())

	u = u.JoinPath(fmt.Sprintf("/api/v1/teams/%s/pipelines/%s/resources/%s/check/webhook",
		pipeline.Team,
		pipeline.Name,
		resource.Name,
	))

	qs := url.Values{}

	if pipeline.InstanceVars != nil {
		qs = atc.PipelineRef{Name: pipeline.Name, InstanceVars: pipeline.InstanceVars}.QueryParams()
	}

	qs.Add("webhook_token", resource.WebhookToken)

	u.RawQuery = qs.Encode()

	return u.String()
}
