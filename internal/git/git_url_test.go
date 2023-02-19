package git_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/matthope/concourse-webhook-broadcaster/internal/git"
)

func TestGitURIComparision(t *testing.T) {
	t.Parallel()

	type args struct {
		URL1 string
		URL2 string
	}

	type want struct {
		Result bool
	}

	tests := map[string]struct {
		Args args
		Want want
	}{
		"1": {
			Args: args{URL1: "https://git.foo/some/repo", URL2: "https://git.foo/other/repo"},
			Want: want{Result: false},
		},
		"2": {
			Args: args{URL1: "https://git.foo/some/repo", URL2: "https://git.foo/some/repo.git"},
			Want: want{Result: true},
		},
		"3": {
			Args: args{URL1: "https://git.foo/some/repo.git", URL2: "git://git.foo/some/repo"},
			Want: want{Result: true},
		},
		"4": {
			Args: args{URL1: "git@git.foo:some/repo.git", URL2: "https://git.foo/some/repo.git"},
			Want: want{Result: true},
		},
		"5": {
			Args: args{URL1: "git@git.foo:some/repo.git", URL2: "nase@git.foo:some/repo.git"},
			Want: want{Result: true},
		},
		"6": {
			Args: args{URL1: "git@git.foo:some/repo.git", URL2: "nase@git.foo:some/repo2.git"},
			Want: want{Result: false},
		},
		"7": {
			Args: args{URL1: "git@git.bar:some/repo.git", URL2: "nase@git.foo:some/repo.git"},
			Want: want{Result: false},
		},
	}

	for name, tt := range tests {
		tt := tt

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := git.SameGitRepository(tt.Args.URL1, tt.Args.URL2)

			assert.Equal(t, tt.Want.Result, got)
		})
	}
}
