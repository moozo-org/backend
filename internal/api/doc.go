package api

import (
	"context"
	"embed"
	"moozo/internal/api/generated"
	"strings"
)

//go:embed bundled/swagger.yaml
var specFile embed.FS

// GetDocumentation implémente l'opération getDocumentation
func (h *Handler) GetDocumentation(ctx context.Context) (generated.GetDocumentationOK, error) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.31.0/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.31.0/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '/docs/openapi.yaml',
            dom_id: '#swagger-ui',
        })
    </script>
</body>
</html>`

	return generated.GetDocumentationOK{Data: strings.NewReader(html)}, nil
}

// GetOpenAPISpec implémente l'opération getOpenAPISpec
func (h *Handler) GetOpenAPISpec(ctx context.Context) (generated.GetOpenAPISpecOK, error) {
	data, err := specFile.ReadFile("bundled/swagger.yaml")
	if err != nil {
		return generated.GetOpenAPISpecOK{}, err
	}

	return generated.GetOpenAPISpecOK{Data: strings.NewReader(string(data))}, nil
}
