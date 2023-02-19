package github

import (
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

func (h *WebhookHandler) matchFiles(patterns []string, files []string) bool {
	return matchFiles(patterns, files, h.Logger)
}

// matchFiles will return true if at least one file matches at least one pattern.
func matchFiles(patterns []string, files []string, logger *zap.Logger) bool {
	for _, file := range files {
		for _, pattern := range patterns {
			// direct match
			if file == pattern {
				logger.Debug("direct match", zap.String("file", file), zap.String("pattern", pattern))

				return true
			}

			// directory match
			if strings.HasSuffix(pattern, "/") && strings.HasPrefix(file, pattern) {
				logger.Debug("directory match", zap.String("file", file), zap.String("pattern", pattern))

				return true
			}

			// directory without trainling / match
			if strings.HasPrefix(file, pattern+"/") {
				logger.Debug("prefix match", zap.String("file", file), zap.String("pattern", pattern))

				return true
			}

			// last resort glob match
			if ok, _ := filepath.Match(pattern, file); ok {
				logger.Debug("glob match", zap.String("file", file), zap.String("pattern", pattern))

				return true
			}
		}
	}

	return false
}
