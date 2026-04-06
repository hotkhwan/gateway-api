// internal/swagger/swagger.go
// Fiber v3 compatible Swagger UI handler.
// Adapted from github.com/gofiber/swagger v1.1.1 for fiber v3.
package swagger

import (
	"fmt"
	"html/template"
	"io"
	"mime"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"
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
	// AssetBase is the base path for static assets (computed from route prefix).
	// Set automatically on first request; do not set manually.
	AssetBase string
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

	return func(c fiber.Ctx) error {
		once.Do(func() {
			prefix = strings.ReplaceAll(c.Route().Path, "*", "")
			if fwdPrefix := getForwardedPrefix(c); fwdPrefix != "" {
				prefix = fwdPrefix + prefix
			}
			// Ensure prefix ends with / so asset URLs are correct.
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			if cfg.URL == "" {
				cfg.URL = prefix + defaultDocURL
			}
			cfg.AssetBase = prefix
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
			// Serve asset directly from the embedded FS.
			f, err := swaggerFiles.FS.Open(p)
			if err != nil {
				return fiber.ErrNotFound
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				return fiber.ErrInternalServerError
			}
			mimeType := mime.TypeByExtension(filepath.Ext(p))
			if mimeType == "" {
				mimeType = fiber.MIMEOctetStream
			}
			c.Set(fiber.HeaderContentType, mimeType)
			return c.Send(data)
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
// AssetBase is an absolute path prefix (e.g. /api/v4/docs/) so assets load
// correctly regardless of whether the URL has a trailing slash.
const indexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>{{.Title}}</title>
  <link rel="stylesheet" type="text/css" href="{{.AssetBase}}swagger-ui.css">
  <link rel="icon" type="image/png" href="{{.AssetBase}}favicon-32x32.png" sizes="32x32"/>
  <link rel="icon" type="image/png" href="{{.AssetBase}}favicon-16x16.png" sizes="16x16"/>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="{{.AssetBase}}swagger-ui-bundle.js"></script>
  <script src="{{.AssetBase}}swagger-ui-standalone-preset.js"></script>
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
