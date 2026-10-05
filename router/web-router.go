package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS         embed.FS
	IndexPage       []byte
	MobileBuildFS   embed.FS
	MobileIndexPage []byte
}

// desktopPreferenceCookie is set by the mobile console when the operator taps
// "Desktop version", so a mobile user agent stops being redirected to /m.
const desktopPreferenceCookie = "newapi_prefer_desktop"

// mobileUserAgentMarkers is a small allowlist of tokens that phone browsers
// ship. iPadOS 13+ reports a desktop Safari user agent and is deliberately not
// matched; such devices stay on the desktop dashboard.
var mobileUserAgentMarkers = []string{
	"Mobile", "Android", "iPhone", "iPod", "Windows Phone", "IEMobile", "BlackBerry", "Opera Mini", "webOS",
}

func isMobileUserAgent(userAgent string) bool {
	for _, marker := range mobileUserAgentMarkers {
		if strings.Contains(userAgent, marker) {
			return true
		}
	}
	return false
}

// prefersDesktopShell reports whether a mobile-looking browser explicitly asked
// for the desktop dashboard, with ?desktop=1 or with the preference cookie.
// The ?desktop=0 opt-out is handled before this check in desktopShellHandler,
// because it must win over the cookie set by a previous ?desktop=1 visit.
func prefersDesktopShell(c *gin.Context) bool {
	if c.Query("desktop") == "1" {
		return true
	}
	cookie, err := c.Cookie(desktopPreferenceCookie)
	return err == nil && cookie == "1"
}

// desktopShellHandler serves the desktop SPA entry and redirects the root path
// of mobile browsers to the mobile console. It runs after static.Serve, so only
// paths with no matching embedded file reach it.
func desktopShellHandler(indexPage []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/" &&
			isMobileUserAgent(c.GetHeader("User-Agent")) {
			if c.Query("desktop") == "0" {
				// The mobile console's "Desktop version" button only knows how to
				// set the preference cookie, so this query parameter is the way
				// back: expire the cookie and send the phone to /m. It is checked
				// before prefersDesktopShell so it also overrides a cookie set by
				// an earlier ?desktop=1 visit.
				c.SetCookie(desktopPreferenceCookie, "", -1, "/", "", false, true)
				c.Redirect(http.StatusFound, "/m")
				return
			}
			if !prefersDesktopShell(c) {
				c.Redirect(http.StatusFound, "/m")
				return
			}
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	}
}

// withWebChain builds the per-request middleware chain shared by the desktop
// fallback and the mobile console. The order matches the pre-existing desktop
// chain exactly, and the chain is never registered with router.Use, so adding
// the mobile routes cannot widen gzip, caching or rate limiting to other
// routes on the engine.
func withWebChain(pluginDispatcher gin.HandlerFunc, extra ...gin.HandlerFunc) []gin.HandlerFunc {
	chain := []gin.HandlerFunc{
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
	}
	return append(chain, extra...)
}

// buildMobileShell serves the mobile console: real files under /m come from the
// mobile build, everything else returns the mobile index so client-side deep
// links keep working. Cache-Control for the index is forced to no-cache because
// middleware.Cache sets a one-week max-age for every non-root path.
func buildMobileShell(fs static.ServeFileSystem, indexPage []byte) gin.HandlerFunc {
	fileServer := http.StripPrefix("/m", http.FileServer(fs))
	return func(c *gin.Context) {
		relativePath := strings.TrimPrefix(c.Request.URL.Path, "/m")
		if relativePath != "" {
			if file, err := fs.Open(relativePath); err == nil {
				_ = file.Close()
				// Go's mime table has no entry for .webmanifest, so a manifest
				// would be sniffed as text/plain and some browsers refuse it.
				if strings.HasSuffix(relativePath, ".webmanifest") {
					c.Header("Content-Type", "application/manifest+json")
				}
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			// Build output lives under /static; a miss there is a genuine 404.
			// Answering it with the HTML shell would make a browser parse the
			// index page as JavaScript or CSS.
			if strings.HasPrefix(relativePath, "/static/") {
				c.Status(http.StatusNotFound)
				return
			}
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	}
}

// SetWebRouter serves the desktop shell and the mobile console. gzip, rate
// limiting and caching stay inside the web chain instead of being registered
// with router.Use, so the set and order of middleware running for other routes
// on the engine is unchanged.
func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	mobileFS := common.EmbedFolder(assets.MobileBuildFS, "web/mobile-dist")
	mobileShell := buildMobileShell(mobileFS, assets.MobileIndexPage)

	router.GET("/m", withWebChain(pluginDispatcher, mobileShell)...)
	router.GET("/m/*path", withWebChain(pluginDispatcher, mobileShell)...)
	router.NoRoute(
		withWebChain(pluginDispatcher,
			static.Serve("/", frontendFS),
			desktopShellHandler(assets.IndexPage),
		)...,
	)
}
