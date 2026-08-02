package schemas

import (
	"embed"
	"fmt"
)

//go:embed *.schema.json
var content embed.FS

func Requirements() ([]byte, error) {
	return read("requirements.schema.json")
}

func Review() ([]byte, error) {
	return read("review.schema.json")
}

func read(name string) ([]byte, error) {
	data, err := content.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read embedded schema %q: %w", name, err)
	}
	return data, nil
}
