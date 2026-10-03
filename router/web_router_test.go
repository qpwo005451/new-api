/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package router

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/web-dist
var testDesktopFS embed.FS

//go:embed testdata/mobile-dist
var testMobileFS embed.FS

const testDesktopIndex = "<!doctype html><title>desktop shell</title>"

const testMobileIndex = "<!doctype html><title>mobile shell</title>"

func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(desktopShellHandler([]byte(testDesktopIndex)))
	return engine
}

func TestIsMobileUserAgent(t *testing.T) {
	cases := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{name: "iPhone Safari", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1", want: true},
		{name: "Android Chrome", userAgent: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36", want: true},
		{name: "Opera Mini", userAgent: "Opera/9.80 (J2ME/MIDP; Opera Mini/9.80) Presto/2.12.423 Version/12.16", want: true},
		{name: "desktop Chrome", userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", want: false},
		{name: "desktop Safari on macOS", userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", want: false},
		{name: "iPadOS 13 desktop UA", userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", want: false},
		{name: "empty", userAgent: "", want: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, isMobileUserAgent(testCase.userAgent))
		})
	}
}

func TestDesktopShellRedirectsMobileRootToMobileConsole(t *testing.T) {
	engine := newTestEngine(t)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148 Safari/604.1")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/m", recorder.Header().Get("Location"))
}

func TestDesktopShellCases(t *testing.T) {
	cases := []struct {
		name         string
		target       string
		userAgent    string
		cookie       string
		wantStatus   int
		wantLocation string
		wantBody     string
	}{
		{name: "desktop root renders desktop shell", target: "/", userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0.0.0 Safari/537.36", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "mobile root with desktop=1 stays on desktop", target: "/?desktop=1", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "mobile root with preference cookie stays on desktop", target: "/", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", cookie: desktopPreferenceCookie + "=1", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "mobile deep link is not redirected", target: "/usage-logs", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "api path returns relay not found", target: "/api/unknown", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", wantStatus: http.StatusNotFound},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestEngine(t)
			request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
			request.Header.Set("User-Agent", testCase.userAgent)
			if testCase.cookie != "" {
				request.Header.Set("Cookie", testCase.cookie)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, testCase.wantStatus, recorder.Code)
			if testCase.wantLocation != "" {
				assert.Equal(t, testCase.wantLocation, recorder.Header().Get("Location"))
			}
			if testCase.wantBody != "" {
				assert.Equal(t, testCase.wantBody, recorder.Body.String())
			}
			if testCase.wantStatus == http.StatusOK {
				assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func newTestMobileEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	shell := buildMobileShell(common.EmbedFolder(testMobileFS, "testdata/mobile-dist"), []byte(testMobileIndex))
	engine.GET("/m", shell)
	engine.GET("/m/*path", shell)
	return engine
}

func TestMobileShellServesIndexAndFallsBackForDeepLinks(t *testing.T) {
	cases := []struct {
		name        string
		target      string
		wantStatus  int
		wantBody    string
		wantNoCache bool
	}{
		{name: "root of mobile console", target: "/m", wantStatus: http.StatusOK, wantBody: testMobileIndex, wantNoCache: true},
		{name: "deep link falls back to mobile index", target: "/m/routing", wantStatus: http.StatusOK, wantBody: testMobileIndex, wantNoCache: true},
		{name: "hashed asset is served from the mobile build", target: "/m/static/app.js", wantStatus: http.StatusOK, wantBody: "console.log('mobile shell asset')\n", wantNoCache: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestMobileEngine(t)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.target, nil))

			require.Equal(t, testCase.wantStatus, recorder.Code)
			assert.Equal(t, testCase.wantBody, recorder.Body.String())
			if testCase.wantNoCache {
				assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			} else {
				assert.Empty(t, recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

const (
	testMobileUserAgent  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148 Safari/604.1"
	testDesktopUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// noopPluginDispatcher stands in for the real plugin dispatcher so the tests
// can assemble the production web router without any plugin state.
func noopPluginDispatcher(c *gin.Context) { c.Next() }

// newTestWebEngine assembles the production web router, chain included, on an
// empty test engine. The embedded file systems are the zero value (an embed.FS
// cannot be fabricated), so every asset lookup misses, but both index pages are
// supplied directly, which lets the response body prove which handler ran.
func newTestWebEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetWebRouter(engine, WebAssets{
		IndexPage:       []byte(testDesktopIndex),
		MobileIndexPage: []byte(testMobileIndex),
	}, noopPluginDispatcher)
	return engine
}

// TestWebRouterServesMobileConsole exercises the real SetWebRouter assembly
// rather than a hand-built /m route, so the route, chain, index-page binding
// and fallback decisions are covered together.
func TestWebRouterServesMobileConsole(t *testing.T) {
	cases := []struct {
		name             string
		method           string
		target           string
		userAgent        string
		wantStatus       int
		wantLocation     string
		wantBody         string
		wantCacheControl string
	}{
		{name: "GET /m serves the mobile index", method: http.MethodGet, target: "/m", wantStatus: http.StatusOK, wantBody: testMobileIndex, wantCacheControl: "no-cache"},
		{name: "deep link falls back to the mobile index", method: http.MethodGet, target: "/m/deep/link", wantStatus: http.StatusOK, wantBody: testMobileIndex, wantCacheControl: "no-cache"},
		{name: "non GET methods fall through to the shared NoRoute shell", method: http.MethodPost, target: "/m", wantStatus: http.StatusOK, wantBody: testDesktopIndex, wantCacheControl: "no-cache"},
		{name: "mobile root redirects to the console", method: http.MethodGet, target: "/", userAgent: testMobileUserAgent, wantStatus: http.StatusFound, wantLocation: "/m", wantCacheControl: "no-cache"},
		{name: "desktop root still falls back to the desktop shell", method: http.MethodGet, target: "/", userAgent: testDesktopUserAgent, wantStatus: http.StatusOK, wantBody: testDesktopIndex, wantCacheControl: "no-cache"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestWebEngine(t)
			request := httptest.NewRequest(testCase.method, testCase.target, nil)
			if testCase.userAgent != "" {
				request.Header.Set("User-Agent", testCase.userAgent)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, testCase.wantStatus, recorder.Code)
			if testCase.wantLocation != "" {
				assert.Equal(t, testCase.wantLocation, recorder.Header().Get("Location"))
			}
			if testCase.wantStatus == http.StatusOK {
				assert.Equal(t, testCase.wantBody, recorder.Body.String())
			}
			assert.Equal(t, testCase.wantCacheControl, recorder.Header().Get("Cache-Control"))
		})
	}
}

// TestWebRouterRegistersMobileConsoleForGETOnly is the structural counterpart to
// the HTTP-table test. It reads the engine route table directly, so widening /m
// to another method fails here even though NoRoute would still answer every
// unmatched request with 200.
func TestWebRouterRegistersMobileConsoleForGETOnly(t *testing.T) {
	engine := newTestWebEngine(t)

	methodsByPath := map[string]map[string]bool{}
	for _, route := range engine.Routes() {
		if route.Path != "/m" && route.Path != "/m/*path" {
			continue
		}
		if methodsByPath[route.Path] == nil {
			methodsByPath[route.Path] = map[string]bool{}
		}
		methodsByPath[route.Path][route.Method] = true
	}

	require.Contains(t, methodsByPath, "/m")
	require.Contains(t, methodsByPath, "/m/*path")
	for _, path := range []string{"/m", "/m/*path"} {
		assert.Equal(t, map[string]bool{http.MethodGet: true}, methodsByPath[path], "%s must only be registered for GET", path)
	}
}

// TestWebChainStaysOffTheEngineScope guards the binding decision that the web
// middleware must only ever be attached per route. Registering gzip, rate
// limiting or caching with router.Use would leak them onto every other route on
// the engine, which this probe detects.
func TestWebChainStaysOffTheEngineScope(t *testing.T) {
	engine := newTestWebEngine(t)
	engine.GET("/__probe", func(c *gin.Context) { c.String(http.StatusOK, "probe") })

	request := httptest.NewRequest(http.MethodGet, "/__probe", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "probe", recorder.Body.String())
	assert.Empty(t, recorder.Header().Get("Cache-Control"), "router.Use(Cache) would leak a one-week cache header onto unrelated routes")
	assert.Empty(t, recorder.Header().Get("Cache-Version"), "router.Use(Cache) would leak the cache version onto unrelated routes")
	assert.Empty(t, recorder.Header().Get("Content-Encoding"), "router.Use(gzip) would compress unrelated routes")
}

// testMobileAssetFS adapts the testdata embed.FS to static.ServeFileSystem. The
// fixture keys carry the testdata/ prefix, so common.EmbedFolder cannot point
// the mobile build at them; this test-only type lets the real withWebChain serve
// a real mobile asset.
type testMobileAssetFS struct {
	http.FileSystem
}

func (f testMobileAssetFS) Exists(prefix string, name string) bool {
	_, err := f.Open(name)
	return err == nil
}

func (f testMobileAssetFS) Open(name string) (http.File, error) {
	if name == "/" {
		return nil, fs.ErrNotExist
	}
	return f.FileSystem.Open(name)
}

// TestWebChainCachesMobileAssetsButNotTheMobileIndex runs a real mobile asset
// and the mobile index through withWebChain, so the interaction between
// middleware.Cache (one week max-age) and the shell's no-cache override is
// asserted on the real chain instead of on a bare engine.
func TestWebChainCachesMobileAssetsButNotTheMobileIndex(t *testing.T) {
	sub, err := fs.Sub(testMobileFS, "testdata/mobile-dist")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	shell := buildMobileShell(testMobileAssetFS{FileSystem: http.FS(sub)}, []byte(testMobileIndex))
	engine.GET("/m", withWebChain(noopPluginDispatcher, shell)...)
	engine.GET("/m/*path", withWebChain(noopPluginDispatcher, shell)...)

	cases := []struct {
		name             string
		target           string
		wantBody         string
		wantCacheControl string
	}{
		{name: "mobile index overrides the one week max-age", target: "/m", wantBody: testMobileIndex, wantCacheControl: "no-cache"},
		{name: "real asset keeps the one week max-age from the web chain", target: "/m/static/app.js", wantBody: "console.log('mobile shell asset')\n", wantCacheControl: "max-age=604800"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.target, nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, testCase.wantBody, recorder.Body.String())
			assert.Equal(t, testCase.wantCacheControl, recorder.Header().Get("Cache-Control"))
		})
	}
}
