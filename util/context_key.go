package util

import "context"

type contextKey string

const RETRY contextKey = "retries"
const BACKOFF contextKey = "backoff"
const IGNORE_TAGS contextKey = "ignore-tags"

// WithIgnoreTags returns a new context with the given tag keys stored as ignored tags.
func WithIgnoreTags(ctx context.Context, keys []string) context.Context {
	return context.WithValue(ctx, IGNORE_TAGS, keys)
}

// IgnoreTagsFromContext retrieves the ignored tag keys stored in the context.
// Returns nil if ctx is nil or no ignored tags are set.
func IgnoreTagsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	tags, _ := ctx.Value(IGNORE_TAGS).([]string)
	return tags
}
