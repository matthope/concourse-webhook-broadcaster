package concourse

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/go-concourse/concourse"
	"golang.org/x/oauth2"
)

var ErrNotFound = errors.New("not found")

const defaultTimeout = 2 * time.Second

type Clients struct {
	Clients []*Client
}

func NewClients(concourseURLs []string) (*Clients, error) {
	c := Clients{
		Clients: make([]*Client, len(concourseURLs)),
	}

	for i := range concourseURLs {
		var err error

		c.Clients[i], err = NewClient(concourseURLs[i])
		if err != nil {
			return nil, err
		}
	}

	return &c, nil
}

type Client struct {
	concourseURL string
	oauth2Config *oauth2.Config
	token        *oauth2.Token
	ctx          context.Context //nolint:containedctx // oauth2 storing in context
}

func NewClient(concourseURL string) (*Client, error) {
	c := Client{concourseURL: concourseURL}

	tokenEndPoint, err := url.Parse("sky/issuer/token")
	if err != nil {
		return &Client{}, fmt.Errorf("url error: %w", err)
	}

	base, err := url.Parse(concourseURL)
	if err != nil {
		return &Client{}, fmt.Errorf("url error %q: %w", concourseURL, err)
	}

	tokenURL := base.ResolveReference(tokenEndPoint)

	/* We leverage the fact that `fly` is considered a "public client" to fetch our oauth token */
	c.oauth2Config = &oauth2.Config{
		ClientID:     "fly",
		ClientSecret: "Zmx5",
		Endpoint:     oauth2.Endpoint{TokenURL: tokenURL.String()},
		Scopes:       []string{"openid", "profile", "email", "federated:id", "groups"},
	}

	httpClient := &http.Client{Timeout: defaultTimeout}
	c.ctx = context.WithValue(context.Background(), oauth2.HTTPClient, &httpClient)

	return &c, nil
}

func (c *Client) RefreshClientWithToken() (concourse.Client, error) {
	if !c.token.Valid() {
		var err error

		username, password := userPassFromURL(c.concourseURL)

		c.token, err = c.oauth2Config.PasswordCredentialsToken(c.ctx, username, password)
		if err != nil {
			return nil, fmt.Errorf("token error: %w", err)
		}
	}

	httpClient := c.oauth2Config.Client(c.ctx, c.token)
	concourseClient := concourse.NewClient(c.concourseURL, httpClient, false)

	return concourseClient, nil
}

func userPassFromURL(concourseURL string) (string, string) {
	u, _ := url.Parse(concourseURL)
	username := u.User.Username()
	password, _ := u.User.Password()

	return username, password
}

func (c *Client) PipelineID(pipeline atc.Pipeline) string {
	return c.URL() + pipeline.TeamName + "/" + pipeline.Ref().String()
}

func (c *Client) URL() string {
	u, err := url.ParseRequestURI(c.concourseURL)
	if err != nil {
		return ""
	}

	u.User = nil

	if u.Path == "" {
		u.Path = "/"
	}

	return u.String()
}

func (c *Clients) FindClient(u string) *Client {
	for _, i := range c.Clients {
		if strings.HasPrefix(u, i.concourseURL) {
			return i
		}
	}

	return nil
}
