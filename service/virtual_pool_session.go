package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	virtualPoolSessionHeader          = "X-NewAPI-Session-ID"
	virtualPoolSessionMaxBytes        = 256
	virtualPoolSessionNamespacePrefix = "new-api:virtual-sticky:v1:"
)

var errVirtualPoolSessionConflict = errors.New("conflicting virtual pool session identifiers")

// VirtualPoolSession is the server-derived affinity identity. Raw session IDs
// stay in memory only; cache keys use digests.
type VirtualPoolSession struct {
	Source        string
	SessionDigest string
	ScopeDigest   string
	CacheKey      string
}

// ResolveVirtualPoolSession extracts a session identity from request headers or
// an allowed body field. A nil session means the request has no usable ID and
// must not participate in sticky routing.
func ResolveVirtualPoolSession(c *gin.Context, modelName string) (*VirtualPoolSession, error) {
	if c == nil || c.Request == nil {
		return nil, nil
	}
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	if !setting.Allows(userID, tokenID, group, modelName) {
		return nil, nil
	}

	sessionID, source, err := extractVirtualPoolSessionID(c, setting)
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, nil
	}

	sessionDigest := common.Sha1([]byte(source + "\x00" + sessionID))
	scope := strings.Join([]string{
		fmt.Sprintf("user=%d", userID),
		fmt.Sprintf("token=%d", tokenID),
		"group=" + strings.ToLower(strings.TrimSpace(group)),
		"model=" + strings.ToLower(strings.TrimSpace(modelName)),
		"mode=" + setting.SessionMode,
		"v=1",
	}, "\x00")
	scopeDigest := common.Sha1([]byte(scope))
	return &VirtualPoolSession{
		Source:        source,
		SessionDigest: sessionDigest,
		ScopeDigest:   scopeDigest,
		CacheKey:      virtualPoolSessionNamespacePrefix + scopeDigest + ":" + sessionDigest,
	}, nil
}

func extractVirtualPoolSessionID(c *gin.Context, setting operation_setting.VirtualPoolStickySetting) (string, string, error) {
	headerSources := []struct {
		source string
		names  []string
	}{
		{source: "x-newapi-session-id", names: []string{virtualPoolSessionHeader}},
	}
	if setting.SessionMode == operation_setting.VirtualPoolSessionModeThread {
		headerSources = append(headerSources,
			struct {
				source string
				names  []string
			}{source: "thread-id", names: []string{"thread-id", "thread_id"}},
		)
	}
	headerSources = append(headerSources,
		struct {
			source string
			names  []string
		}{source: "session-id", names: []string{"session-id", "session_id"}},
	)

	for _, candidate := range headerSources {
		value, present, err := readVirtualPoolHeaderAliases(c, candidate.source, candidate.names)
		if err != nil {
			return "", "", err
		}
		if !present {
			continue
		}
		if err := validateVirtualPoolSessionID(value); err != nil {
			return "", "", err
		}
		return value, candidate.source, nil
	}

	bodyID, bodySource, err := extractVirtualPoolSessionFromBody(c, setting)
	if err != nil {
		return "", "", err
	}
	if bodyID != "" {
		return bodyID, bodySource, nil
	}
	return "", "", nil
}

func readVirtualPoolHeaderAliases(c *gin.Context, source string, names []string) (string, bool, error) {
	var found string
	for _, name := range names {
		values := c.Request.Header.Values(name)
		for _, raw := range values {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if found != "" && found != value {
				return "", false, fmt.Errorf("%w: %s", errVirtualPoolSessionConflict, source)
			}
			found = value
		}
	}
	return found, found != "", nil
}

func extractVirtualPoolSessionFromBody(c *gin.Context, setting operation_setting.VirtualPoolStickySetting) (string, string, error) {
	if c.Request == nil || c.Request.Body == nil {
		return "", "", nil
	}
	contentType := c.Request.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		return "", "", nil
	}
	var body struct {
		SessionID      any `json:"session_id"`
		PromptCacheKey any `json:"prompt_cache_key"`
	}
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return "", "", err
	}
	sessionID, _ := body.SessionID.(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		if err := validateVirtualPoolSessionID(sessionID); err != nil {
			return "", "", err
		}
		return sessionID, "body-session-id", nil
	}
	if setting.AllowPromptCacheKey {
		promptCacheKey, _ := body.PromptCacheKey.(string)
		promptCacheKey = strings.TrimSpace(promptCacheKey)
		if promptCacheKey != "" {
			if err := validateVirtualPoolSessionID(promptCacheKey); err != nil {
				return "", "", err
			}
			return promptCacheKey, "prompt-cache-key", nil
		}
	}
	return "", "", nil
}

func validateVirtualPoolSessionID(value string) error {
	if value == "" {
		return nil
	}
	if !utf8.ValidString(value) {
		return errors.New("virtual pool session id must be valid UTF-8")
	}
	if len([]byte(value)) > virtualPoolSessionMaxBytes {
		return fmt.Errorf("virtual pool session id exceeds %d bytes", virtualPoolSessionMaxBytes)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return errors.New("virtual pool session id contains control characters")
		}
	}
	return nil
}

// AbortVirtualPoolSessionError returns a stable 400 response for malformed IDs.
func AbortVirtualPoolSessionError(c *gin.Context, err error) {
	if c == nil || err == nil {
		return
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"error": gin.H{
			"message": err.Error(),
			"type":    "invalid_request_error",
			"code":    "virtual_pool_session_invalid",
		},
	})
}
