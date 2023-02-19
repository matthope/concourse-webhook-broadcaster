package resourceparser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/concourse/concourse/atc"
)

// The various GitHub-related resource types store their repository
// configuration in different ways. This function handles the different
// approaches and returns a standardised repository URL.
func ConstructURIFromConfig(resource atc.ResourceConfig) (string, bool) {
	source := sourceToMap(resource.Source)

	if len(source) == 0 {
		return "", false
	}

	if uri, found := source["url"]; found {
		if uri == "https://api.bitbucket.org" {
			return bitbucketRef(source)
		}

		return uri, true
	}

	if uri, found := source["uri"]; found {
		if isURL(uri) {
			return uri, true
		}
	}

	if uri, found := source["repository"]; found {
		if isURL(uri) {
			return uri, true
		}
	}

	return "", false
}

func sourceToMap(s atc.Source) map[string]string {
	out := make(map[string]string, len(s))

	for k := range s {
		if val, ok := s[k].(string); ok {
			if strings.HasPrefix(val, `((`) {
				continue // ignore keys that have secrets/params as values.
			}

			out[k] = convertGitURL(val)
		}
	}

	return out
}

func bitbucketRef(source map[string]string) (string, bool) {
	if contains(source, "team") && contains(source, "repo") {
		return fmt.Sprintf("https://bitbucket.org/%s/%s", source["team"], source["repo"]), true
	}

	return "", false
}

func contains(s map[string]string, e string) bool {
	_, found := s[e]

	return found
}

func isURL(s string) bool {
	_, err := url.ParseRequestURI(s)

	return err == nil
}

func convertGitURL(s string) string {
	gitURIRegex := regexp.MustCompile(`(https?://|git://|[^@]+@)(?P<host>[-.a-z0-9]+)[:/](?P<repository>.*)`)

	if !gitURIRegex.MatchString(s) {
		return s
	}

	out := gitURIRegex.FindStringSubmatch(s)

	return fmt.Sprintf("https://%s/%s", out[2], out[3])
}
