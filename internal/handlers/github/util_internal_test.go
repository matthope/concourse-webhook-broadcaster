package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchFiles(t *testing.T) {
	t.Parallel()

	type args struct {
		Patterns []string
		Files    []string
	}

	type want struct {
		Result bool
	}

	tests := map[string]struct {
		Args args
		Want want
	}{
		"1": {
			Args: args{Patterns: []string{"ap-ae-1/values/globals.yaml"}, Files: []string{"qa-de-1/values/designate.yaml"}},
			Want: want{Result: false},
		},
	}
	for name, tt := range tests {
		tt := tt

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := &WebhookHandler{}

			got := h.matchFiles(tt.Args.Patterns, tt.Args.Files)

			assert.Equal(t, tt.Want.Result, got)
		})
	}
}
