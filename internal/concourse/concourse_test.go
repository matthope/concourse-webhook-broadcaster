package concourse_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	concourse "github.com/matthope/concourse-webhook-broadcaster/internal/concourse"
)

func TestClient_URL(t *testing.T) {
	t.Parallel()

	type fields struct {
		concourseURL string
	}

	tests := map[string]struct {
		fields fields
		want   string
	}{
		"empty": {},
		"with trailing slash": {
			fields: fields{concourseURL: "https://localhost:9090/"},
			want:   "https://localhost:9090/",
		},
		"without trailing slash": {
			fields: fields{concourseURL: "https://localhost:9090"},
			want:   "https://localhost:9090/",
		},
	}
	for name, tt := range tests {
		tt := tt

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, _ := concourse.NewClient(tt.fields.concourseURL)

			got := c.URL()

			assert.Equal(t, tt.want, got)
		})
	}
}
