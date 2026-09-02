package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestHandlerServesEmbeddedApplicationAndAssets(t *testing.T) {
	handler := NewHandler(http.NotFoundHandler())

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), "<title>Semlia</title>") {
		t.Fatalf("index response = %d %q", index.Code, index.Body.String())
	}
	if got := index.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("index cache control = %q", got)
	}

	assetPath := regexp.MustCompile(`src="([^"]+)"`).FindStringSubmatch(index.Body.String())
	if len(assetPath) != 2 {
		t.Fatal("index does not reference a script asset")
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, assetPath[1], nil))
	if asset.Code != http.StatusOK || asset.Body.Len() == 0 {
		t.Fatalf("asset response = %d, bytes = %d", asset.Code, asset.Body.Len())
	}
	if got := asset.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset cache control = %q", got)
	}
}

func TestHandlerDoesNotUseApplicationFallbackForMissingAssets(t *testing.T) {
	handler := NewHandler(http.NotFoundHandler())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing asset response = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestHandlerRoutesAPIPrefixesAndFallsBackForClientRoutes(t *testing.T) {
	api := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusTeapot)
	})
	handler := NewHandler(api)

	for _, path := range []string{"/health/live", "/api/v1/system/info"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusTeapot {
			t.Fatalf("%s response = %d, want %d", path, response.Code, http.StatusTeapot)
		}
	}

	clientRoute := httptest.NewRecorder()
	handler.ServeHTTP(clientRoute, httptest.NewRequest(http.MethodGet, "/future/route", nil))
	if clientRoute.Code != http.StatusOK || !strings.Contains(clientRoute.Body.String(), "<title>Semlia</title>") {
		t.Fatalf("client route response = %d %q", clientRoute.Code, clientRoute.Body.String())
	}
}
