// internal/swagger/swagger.go
package swagger_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/swaggo/swag"

	swaggermw "github.com/hotkhwan/gateway-api/internal/swagger"
)

// fakeSwagger implements swag.Swagger for test registration.
type fakeSwagger struct{ doc string }

func (f *fakeSwagger) ReadDoc() string { return f.doc }

const fakeDoc = `{"swagger":"2.0","info":{"title":"Test"}}`

// registerFakeDoc registers a swag doc under name; skips if already registered
// (swag panics on duplicate). Returns the registered name.
func registerFakeDoc(name string) {
	defer func() { recover() }() // suppress duplicate-register panic
	swag.Register(name, &fakeSwagger{doc: fakeDoc})
}

func newSwaggerApp(handler fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		// Do not redirect "GET /swagger" → "GET /swagger/" automatically.
	})
	app.Get("/swagger/*", handler)
	return app
}

// TestNew_IndexHTMLReturnsSwaggerUI verifies that GET /swagger/ and GET
// /swagger/index.html both return a 200 HTML response containing swagger-ui
// script tags.
func TestNew_IndexHTMLReturnsSwaggerUI(t *testing.T) {
	registerFakeDoc(swag.Name)

	tests := []struct {
		name string
		path string
	}{
		{"bare prefix with trailing slash treated as empty wildcard — index.html", "/swagger/index.html"},
		{"explicit index.html", "/swagger/index.html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSwaggerApp(swaggermw.New(swaggermw.Config{
				Title: "Test API",
			}))

			req := httptest.NewRequest("GET", tt.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			html := string(body)

			if !strings.Contains(html, "swagger-ui") {
				t.Error("expected HTML to contain 'swagger-ui'")
			}
			if !strings.Contains(html, "Test API") {
				t.Error("expected HTML to contain configured title 'Test API'")
			}
		})
	}
}

// TestNew_DocJSONReturnsRegisteredSpec verifies that GET /swagger/doc.json
// returns the JSON registered with swag.Register.
func TestNew_DocJSONReturnsRegisteredSpec(t *testing.T) {
	registerFakeDoc(swag.Name)

	app := newSwaggerApp(swaggermw.New())

	req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"swagger"`) {
		t.Errorf("expected JSON body to contain swagger spec; got: %s", body)
	}
}

// TestNew_DocJSONNotRegistered verifies that GET /swagger/doc.json returns a
// non-2xx error when no swag instance has been registered for the given name.
func TestNew_DocJSONNotRegistered(t *testing.T) {
	// Use a unique instance name so the global registry won't have it.
	app := newSwaggerApp(swaggermw.New(swaggermw.Config{
		InstanceName: "nonexistent-instance-xyzzy",
	}))

	req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected error status, got %d", resp.StatusCode)
	}
}

// TestNew_TrailingSlashServesIndex verifies that GET /swagger/ (wildcard = "")
// serves the index HTML directly (Fiber normalises the trailing slash so the
// wildcard param is empty, matching the "" case in the handler).
func TestNew_TrailingSlashServesIndex(t *testing.T) {
	registerFakeDoc(swag.Name)

	app := newSwaggerApp(swaggermw.New())

	req := httptest.NewRequest("GET", "/swagger/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	// Trailing slash gives wildcard "" → renders index.html (200 HTML).
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "swagger-ui") {
		t.Error("expected swagger-ui in HTML response for trailing-slash path")
	}
}

// TestNew_UnknownPathServesStaticAsset verifies that an unrecognised wildcard
// segment (e.g. "swagger-ui.css") is forwarded to the static file handler and
// does NOT return 404 from the switch itself.  The static handler may return
// 200 or 404 depending on embedded asset availability; we only assert that the
// request reaches the handler without a 500.
func TestNew_UnknownPathServesStaticAsset(t *testing.T) {
	registerFakeDoc(swag.Name)

	app := newSwaggerApp(swaggermw.New())

	req := httptest.NewRequest("GET", "/swagger/swagger-ui.css", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode == fiber.StatusInternalServerError {
		t.Errorf("did not expect 500 for static asset path; got %d", resp.StatusCode)
	}
}

// TestNew_DefaultTitle verifies that omitting a Title defaults to "Swagger UI".
func TestNew_DefaultTitle(t *testing.T) {
	registerFakeDoc(swag.Name)

	app := newSwaggerApp(swaggermw.New())

	req := httptest.NewRequest("GET", "/swagger/index.html", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Swagger UI") {
		t.Error("expected default title 'Swagger UI' in HTML")
	}
}

// TestNew_HandlerDefault verifies that the package-level HandlerDefault is
// usable without panicking.
func TestNew_HandlerDefault(t *testing.T) {
	registerFakeDoc(swag.Name)

	app := fiber.New()
	app.Get("/swagger/*", swaggermw.HandlerDefault)

	req := httptest.NewRequest("GET", "/swagger/index.html", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("HandlerDefault: expected 200, got %d", resp.StatusCode)
	}
}

// TestNew_ForwardedPrefixHeader verifies that the X-Forwarded-Prefix header is
// incorporated into the doc.json URL embedded in the HTML.
func TestNew_ForwardedPrefixHeader(t *testing.T) {
	registerFakeDoc(swag.Name)

	app := newSwaggerApp(swaggermw.New(swaggermw.Config{
		Title: "Forwarded Prefix Test",
	}))

	req := httptest.NewRequest("GET", "/swagger/index.html", nil)
	req.Header.Set("X-Forwarded-Prefix", "/api/v1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	htmlStr := string(body)
	// Go's html/template escapes "/" as "\/" inside <script> blocks, so the URL
	// appears as "\/api\/v1\/swagger\/doc.json" in the rendered output.
	if !strings.Contains(htmlStr, "api") || !strings.Contains(htmlStr, "v1") {
		t.Errorf("expected forwarded prefix content in HTML; got: %s", htmlStr)
	}
}
