package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
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
	for _, target := range []string{
		"/assets/missing.js", "/assets/missing.css", "/assets/missing.woff2",
		"/assets/missing.js?revision=rev_missing&release=rls_missing",
		"/assets/ast_missing", "/assets/rev_01m3ev2qq8fvxtz73paszba169",
		"/assets/ast_00000000000000000000000000",
		"/assets/ast_01m3ev2qq8fvxtz73paszba169.js",
		"/assets/ast_01m3ev2qq8fvxtz73paszba169/styles.css",
		"/assets/ast_01m3ev2qq8fvxtz73paszba169/unknown",
		"/assets/ast_01m3ev2qq8fvxtz73paszba169/versions/extra",
		"/assets/ignored/../ast_01m3ev2qq8fvxtz73paszba169",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+target, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, target, nil))
				if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "<title>Semlia</title>") {
					t.Fatalf("missing asset response = %d, want 404 without application fallback", response.Code)
				}
				if strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
					t.Fatal("missing asset must not receive immutable caching")
				}
			})
		}
	}
}

func TestHandlerStaticAssetsKeepPriorityAndCaching(t *testing.T) {
	files := fstest.MapFS{
		"index.html":                            {Data: []byte("<title>Semlia</title>")},
		"assets/app.js":                         {Data: []byte("console.log('synthetic')")},
		"assets/app.css":                        {Data: []byte("body { color: black; }")},
		"assets/app.woff2":                      {Data: []byte("synthetic font")},
		"assets/ast_01m3ev2qq8fvxtz73paszba169": {Data: []byte("static file wins")},
	}
	handler := &handler{api: http.NotFoundHandler(), assets: files, fileServer: http.FileServer(http.FS(files))}
	for name, file := range files {
		if name == "index.html" {
			continue
		}
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+"/"+name, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, "/"+name, nil))
				if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("static response = %d, headers = %v", response.Code, response.Header())
				}
				if method == http.MethodGet && response.Body.String() != string(file.Data) {
					t.Fatal("existing static file was replaced by the application")
				}
				if method == http.MethodHead && response.Body.Len() != 0 {
					t.Fatal("HEAD returned a static asset body")
				}
			})
		}
	}
}

func TestHandlerRejectsWritesToClientAndStaticRoutes(t *testing.T) {
	handler := NewHandler(http.NotFoundHandler())
	for _, target := range []string{"/", "/assets/ast_01m3ev2qq8fvxtz73paszba169", "/assets/ast_01m3ev2qq8fvxtz73paszba169/versions", "/assets/missing.js"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
			t.Run(method+target, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, target, nil))
				if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
					t.Fatalf("write response = %d, Allow = %q", response.Code, response.Header().Get("Allow"))
				}
			})
		}
	}
}

func TestHandlerPreservesAPIAuthorizationAndHealthResponses(t *testing.T) {
	for _, target := range []string{"/api", "/api/v1/system/info", "/api/v1/workspaces/example/catalog/assets/example", "/health", "/health/live", "/health/ready"} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
			for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable} {
				t.Run(method+target+http.StatusText(status), func(t *testing.T) {
					calls := 0
					request := httptest.NewRequest(method, target+"?trace=synthetic", nil)
					handler := NewHandler(http.HandlerFunc(func(response http.ResponseWriter, got *http.Request) {
						calls++
						if got != request {
							t.Fatal("API request was replaced")
						}
						response.Header().Set("X-API-Evidence", "unchanged")
						response.WriteHeader(status)
					}))
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					if calls != 1 || response.Code != status || response.Header().Get("X-API-Evidence") != "unchanged" || response.Header().Get("Cache-Control") != "" || response.Body.Len() != 0 {
						t.Fatalf("API response replaced: calls = %d, status = %d, headers = %v", calls, response.Code, response.Header())
					}
				})
			}
		}
	}
}

func TestHandlerServesAssetClientDeepLinks(t *testing.T) {
	apiCalls := 0
	handler := NewHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { apiCalls++ }))
	const asset = "ast_01m3ev2qq8fvxtz73paszba169"
	for _, target := range []string{
		"/assets/" + asset,
		"/assets/" + asset + "/versions",
		"/assets/" + asset + "?revision=rev_01m3ev2rhdfw2vcm1atafxx7vd&release=rls_01m3f2pbg0f1br2ba4hbtgjgrs",
		"/assets/" + asset + "/versions?revision=rev_01m3ev2rhdfw2vcm1atafxx7vd&release=rls_01m3f2pbg0f1br2ba4hbtgjgrs",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+target, func(t *testing.T) {
				request := httptest.NewRequest(method, target, nil)
				originalURL := request.URL.String()
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("asset client route response = %d, want 200", response.Code)
				}
				if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("asset client route headers = %v", response.Header())
				}
				if method == http.MethodGet && !strings.Contains(response.Body.String(), "<title>Semlia</title>") {
					t.Fatal("asset client route did not serve the embedded application")
				}
				if method == http.MethodHead && response.Body.Len() != 0 {
					t.Fatal("HEAD returned an application body")
				}
				if request.URL.String() != originalURL || response.Header().Get("Location") != "" {
					t.Fatal("asset route or immutable query pins were rewritten")
				}
			})
		}
	}
	if apiCalls != 0 {
		t.Fatalf("asset client routes called the API handler %d times", apiCalls)
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
