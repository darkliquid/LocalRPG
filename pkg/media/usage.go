package media

// Usage is a provider's reported consumption in the media layer's own terms, so
// pkg/media never imports pkg/harness. The service converts it at the boundary.
type Usage struct {
	Characters   int
	InputTokens  int
	OutputTokens int
	Requests     int
	Estimated    bool
}

// UsageReporter is implemented by media clients that can report what the last
// call consumed.
type UsageReporter interface {
	LastUsage() Usage
}
