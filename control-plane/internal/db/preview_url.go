package db

import (
	"fmt"
	"strings"
)

// PreviewHostname builds the ephemeral PR preview host (S14).
func PreviewHostname(projectSlug string, prNumber int32) string {
	slug := strings.ToLower(strings.TrimSpace(projectSlug))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	if slug == "" {
		slug = "app"
	}
	if prNumber <= 0 {
		prNumber = 1
	}
	return fmt.Sprintf("pr-%d.%s.preview.localhost", prNumber, slug)
}

// PreviewPublicURL returns the local edge HTTPS URL for a preview.
func PreviewPublicURL(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return ""
	}
	return DomainPublicURL(hostname)
}
