// internal/swagger/swagger.go
// Fiber v3 compatible Swagger UI handler.
// Adapted from github.com/gofiber/swagger v1.1.1 for fiber v3.
package swagger

import (
	"fmt"
	"html/template"
	"path"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	swaggerFiles "github.com/swaggo/files/v2"
	"github.com/swaggo/swag"
)

const (
	defaultDocURL = "doc.json"
	defaultIndex  = "index.html"
)

// Config stores Swagger UI configuration.
type Config struct {
	// InstanceName is the swag instance name (empty = default).
	InstanceName string
	// Title is the HTML page title.
	Title string
	// URL overrides the doc.json URL shown in Swagger UI.
	URL string
}

// HandlerDefault is a default handler instance.
var HandlerDefault = New()

// New returns a fiber v3 handler that serves Swagger UI.
func New(config ...Config) fiber.Handler {
	cfg := Config{Title: "Swagger UI"}
	if len(config) > 0 {
		cfg = config[0]
		if cfg.Title == "" {
			cfg.Title = "Swagger UI"
		}
	}

	tpl, err := template.New("swagger_index.html").Parse(indexTmpl)
	if err != nil {
		panic(fmt.Errorf("swagger: failed to parse index template: %w", err))
	}

	var (
		prefix string
		once   sync.Once
	)

	// Static file server for swagger-ui assets (bundle, css, etc.)
	fsHandler := static.New("", static.Config{FS: swaggerFiles.FS})

	return func(c fiber.Ctx) error {
		once.Do(func() {
			prefix = strings.ReplaceAll(c.Route().Path, "*", "")
			if fwdPrefix := getForwardedPrefix(c); fwdPrefix != "" {
				prefix = fwdPrefix + prefix
			}
			if cfg.URL == "" {
				cfg.URL = path.Join(prefix, defaultDocURL)
			}
		})

		p := c.Params("*")

		switch p {
		case defaultIndex, "":
			c.Type("html")
			return tpl.Execute(c, cfg)
		case defaultDocURL:
			doc, docErr := swag.ReadDoc(cfg.InstanceName)
			if docErr != nil {
				return docErr
			}
			return c.Type("json").SendString(doc)
		case "/":
			return c.Redirect().Status(fiber.StatusMovedPermanently).To(path.Join(prefix, defaultIndex))
		default:
			// Set the path to the matched wildcard segment so static can find the file.
			c.Path(p)
			return fsHandler(c)
		}
	}
}

func getForwardedPrefix(c fiber.Ctx) string {
	headers := c.GetReqHeaders()["X-Forwarded-Prefix"]
	if len(headers) == 0 {
		return ""
	}
	prefix := ""
	for _, raw := range headers {
		end := len(raw)
		for end > 1 && raw[end-1] == '/' {
			end--
		}
		prefix += raw[:end]
	}
	return prefix
}

// indexTmpl is the minimal Swagger UI HTML page template.
const indexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>{{.Title}}</title>
  <link rel="stylesheet" type="text/css" href="./swagger-ui.css">
  <link rel="icon" type="image/png" href="./favicon-32x32.png" sizes="32x32"/>
  <link rel="icon" type="image/png" href="./favicon-16x16.png" sizes="16x16"/>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="./swagger-ui-bundle.js"></script>
  <script src="./swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: "{{.URL}}",
        dom_id: '#swagger-ui',
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        plugins: [SwaggerUIBundle.plugins.DownloadUrl],
        layout: "StandaloneLayout",
        deepLinking: true
      });
    }
  </script>
</body>
</html>
`

