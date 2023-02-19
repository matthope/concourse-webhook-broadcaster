package main

import (
	"context"
	"os"

	"github.com/matthope/concourse-webhook-broadcaster/cmd/server"
)

func main() {
	ctx := context.Background()

	if err := server.Execute(ctx); err != nil {
		os.Exit(1)
	}
}

// TODO: read file for secrets?
