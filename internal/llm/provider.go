package llm

import "context"

type Provider interface {
	Name() string
	Complete(ctx context.Context, model string, prompt string) (string, error)
	Healthy(ctx context.Context) bool
}