package server

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

type Params struct {
	ConcourseURL       []string      ``
	ExtListenAddr      string        `default:":8080"`
	IntListenAddr      string        `default:":8088"`
	RefreshInterval    time.Duration `default:"30m"`
	WebhookConcurrency int           `default:"20"`
	Debug              bool          `default:"false"`
}

var ErrBadURL = errors.New("bad url")

func (p *Params) IsValid() error {
	if len(p.ConcourseURL) == 0 {
		return fmt.Errorf("%w: URL not specified", ErrBadURL)
	}

	for i := range p.ConcourseURL {
		u, err := url.ParseRequestURI(p.ConcourseURL[i])
		if err != nil {
			return fmt.Errorf("%w %q: %w", ErrBadURL, p.ConcourseURL[i], err)
		}

		if u.User.Username() == "" {
			return fmt.Errorf("%w %q: no username", ErrBadURL, p.ConcourseURL[i])
		}

		if _, ok := u.User.Password(); !ok {
			return fmt.Errorf("%w %q: no password", ErrBadURL, p.ConcourseURL[i])
		}
	}

	return nil
}
