package app

import (
	"fmt"
	"io"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/httpapi"
)

// runOpenAPI renders the OpenAPI 3.1 document to w WITHOUT booting the server or touching any
// dependency — it is piped to api/openapi.yaml (go:generate) and fed to openapi-typescript. The Go
// handler structs are the single source of truth for the contract (docs/architecture §7.5).
func runOpenAPI(w io.Writer) error {
	srv := httpapi.NewServer(httpapi.Deps{})
	spec, err := srv.OpenAPIYAML()
	if err != nil {
		return fmt.Errorf("render openapi: %w", err)
	}
	if _, err := w.Write(spec); err != nil {
		return fmt.Errorf("write openapi: %w", err)
	}
	return nil
}
