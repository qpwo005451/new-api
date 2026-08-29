package operation_setting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ClientAliasOptionKey is the dotted option key persisting the UA alias map.
const ClientAliasOptionKey = "log_client_alias_setting.aliases"

const (
	MaxClientAliasEntries         = 200
	MaxClientAliasUserAgentLength = 512
	MaxClientAliasNameLength      = 60
)

// LogClientAliasSetting maps exact User-Agent strings to admin-defined
// display names for client tools that cannot be recognized automatically.
// Stored in the options table, so aliases apply retroactively to existing
// log entries at display time and sync across nodes.
type LogClientAliasSetting struct {
	Aliases map[string]string `json:"aliases"`
}

var logClientAliasSetting = LogClientAliasSetting{Aliases: map[string]string{}}

func init() {
	config.GlobalConfig.Register("log_client_alias_setting", &logClientAliasSetting)
}

// GetClientAliases returns the configured alias map. Callers must treat the
// returned map as read-only.
func GetClientAliases() map[string]string {
	if logClientAliasSetting.Aliases == nil {
		return map[string]string{}
	}
	return logClientAliasSetting.Aliases
}

// NormalizeClientAliases returns a copy of current with the alias for
// userAgent upserted to name; an empty name removes the entry. Invalid input
// yields an error and leaves current untouched.
func NormalizeClientAliases(current map[string]string, userAgent, name string) (map[string]string, error) {
	userAgent = strings.TrimSpace(userAgent)
	name = strings.TrimSpace(name)
	if userAgent == "" {
		return nil, errors.New("user agent is required")
	}
	if len([]rune(userAgent)) > MaxClientAliasUserAgentLength {
		return nil, fmt.Errorf("user agent exceeds %d characters", MaxClientAliasUserAgentLength)
	}
	if len([]rune(name)) > MaxClientAliasNameLength {
		return nil, fmt.Errorf("name exceeds %d characters", MaxClientAliasNameLength)
	}

	next := make(map[string]string, len(current)+1)
	for ua, existing := range current {
		next[ua] = existing
	}
	if name == "" {
		delete(next, userAgent)
		return next, nil
	}
	if _, exists := next[userAgent]; !exists && len(next) >= MaxClientAliasEntries {
		return nil, fmt.Errorf("too many client aliases (max %d)", MaxClientAliasEntries)
	}
	next[userAgent] = name
	return next, nil
}
