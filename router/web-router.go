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
	BuildFS   embed.FS
	IndexPage []byte
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
			isMobileUserAgent(c.GetHeader("User-Agent")) && !prefersDesktopShell(c) {
			c.Redirect(http.StatusFound, "/m")
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	}
}

// SetWebRouter serves the desktop shell. gzip, rate limiting and caching stay
// inside the web chain instead of being registered with router.Use, so the set
// and order of middleware running for other routes on the engine is unchanged.
func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		static.Serve("/", frontendFS),
		desktopShellHandler(assets.IndexPage),
	)
}
