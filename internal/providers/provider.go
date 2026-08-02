package providers

import (
	"context"

	"github.com/hironeko/loop-engine/internal/domain"
)

type Job struct {
	ID       string
	Provider domain.Provider
	Role     domain.Role
	Project  string
	JobDir   string
	Prompt   []byte
	Schema   []byte
}

type Result struct {
	StructuredOutput []byte
	Stdout           []byte
	Stderr           []byte
}

type Gateway interface {
	Execute(context.Context, Job) (Result, error)
}
