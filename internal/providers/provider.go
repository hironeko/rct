package providers

import (
	"context"

	"github.com/hironeko/rct/internal/domain"
)

type AccessMode string

const (
	AccessReadOnly       AccessMode = "read-only"
	AccessWorkspaceWrite AccessMode = "workspace-write"
)

type Job struct {
	ID       string
	Provider domain.Provider
	Role     domain.Role
	Project  string
	JobDir   string
	Prompt   []byte
	Schema   []byte
	Access   AccessMode
}

type Result struct {
	StructuredOutput []byte
	Stdout           []byte
	Stderr           []byte
}

type Gateway interface {
	Execute(context.Context, Job) (Result, error)
}
