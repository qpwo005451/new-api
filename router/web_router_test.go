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
