package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

type balanceProtectionTaskSummary struct {
	Checked   int `json:"checked"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Protected int `json:"protected"`
	Recovered int `json:"recovered"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
}

func supportsChannelBalanceQuery(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	if isOfficialDeepSeekBalanceChannel(channel) {
		return true
	}
	switch channel.Type {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeCustom,
		constant.ChannelTypeAIProxy,
		constant.ChannelTypeAPI2GPT,
		constant.ChannelTypeAIGC2D,
		constant.ChannelTypeSiliconFlow,
		constant.ChannelTypeOpenRouter,
		constant.ChannelTypeMoonshot:
		return true
	default:
		return false
	}
}

func attachChannelBalanceProtections(channels []*model.Channel) {
	channelIds := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelIds = append(channelIds, channel.Id)
		}
	}
	protections, err := model.GetChannelBalanceProtections(channelIds)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to attach channel balance protections: %v", err))
		protections = map[int]*model.ChannelBalanceProtection{}
	}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channel.BalanceProtection = protections[channel.Id].ToView(supportsChannelBalanceQuery(channel))
	}
}

func attachChannelBalanceProtection(channel *model.Channel) {
	if channel == nil {
		return
	}
	attachChannelBalanceProtections([]*model.Channel{channel})
}

func notifyBalanceProtectionTransition(channel *model.Channel, transition *model.BalanceProtectionTransition) {
	if channel == nil || transition == nil || transition.Before == nil || transition.After == nil {
		return
	}
	if transition.Before.State == transition.After.State || !transition.After.NotifyEnabled {
		return
	}
	subject := fmt.Sprintf("通道「%s」（#%d）余额保护状态变化", channel.Name, channel.Id)
	content := fmt.Sprintf(
		"通道「%s」（#%d）余额保护状态由 %s 变为 %s，当前余额：%.8f，连续查询失败：%d",
		channel.Name,
		channel.Id,
		transition.Before.State,
		transition.After.State,
		channel.Balance,
		transition.After.ConsecutiveFailures,
	)
	service.NotifyRootUser(
		fmt.Sprintf("%s_balance_protection_%d_%s", dto.NotifyTypeChannelUpdate, channel.Id, transition.After.State),
		subject,
		content,
	)
}

func persistBalanceProtectionCheck(channel *model.Channel, balance *float64, checkErr error) (*model.BalanceProtectionTransition, error) {
	if channel == nil {
		return nil, nil
	}
	errorMessage := ""
	if checkErr != nil {
		errorMessage = checkErr.Error()
	}
	transition, err := model.RecordChannelBalanceProtectionCheck(channel.Id, balance, errorMessage)
	if err != nil {
		return nil, err
	}
	if transition != nil && transition.After != nil {
		model.CacheUpdateChannelBalanceProtection(transition.After)
		notifyBalanceProtectionTransition(channel, transition)
	}
	return transition, nil
}

func checkChannelBalanceWithProtection(channel *model.Channel) (channelBalanceResult, error) {
	result, checkErr := updateChannelBalance(channel)
	if checkErr != nil {
		if _, err := persistBalanceProtectionCheck(channel, nil, checkErr); err != nil {
			common.SysLog(fmt.Sprintf("failed to record balance protection check failure: channel_id=%d error=%v", channel.Id, err))
		}
		return channelBalanceResult{}, checkErr
	}
	if result.RawResponse != "" {
		// Balance payload was not a recognizable number; record an unknown-balance check.
		if _, err := persistBalanceProtectionCheck(channel, nil, nil); err != nil {
			return result, err
		}
		return result, nil
	}
	balance := result.Balance
	channel.Balance = balance
	channel.BalanceUpdatedTime = common.GetTimestamp()
	if _, err := persistBalanceProtectionCheck(channel, &balance, nil); err != nil {
		return result, err
	}
	return result, nil
}

func activateBalanceProtectionForChannel(channel *model.Channel, reason string) bool {
	if channel == nil {
		return false
	}
	transition, err := model.ActivateChannelBalanceProtection(channel.Id, reason)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to activate balance protection: channel_id=%d error=%v", channel.Id, err))
		return false
	}
	if transition == nil || transition.After == nil {
		return false
	}
	model.CacheUpdateChannelBalanceProtection(transition.After)
	notifyBalanceProtectionTransition(channel, transition)
	return true
}

func runBalanceProtectionTaskOnce(ctx context.Context, report func(processed, total int)) (balanceProtectionTaskSummary, error) {
	summary := balanceProtectionTaskSummary{}
	channels, err := model.ListDueBalanceProtectionChannels(common.GetTimestamp())
	if err != nil {
		return summary, err
	}
	total := len(channels)
	for index, channel := range channels {
		if ctx.Err() != nil {
			summary.Skipped += total - index
			break
		}
		if channel == nil || channel.ChannelInfo.IsMultiKey || !supportsChannelBalanceQuery(channel) {
			summary.Skipped++
			if report != nil {
				report(index+1, total)
			}
			continue
		}
		before, _ := model.GetChannelBalanceProtection(channel.Id)
		_, checkErr := checkChannelBalanceWithProtection(channel)
		after, _ := model.GetChannelBalanceProtection(channel.Id)
		summary.Checked++
		if checkErr != nil {
			summary.Failed++
		} else {
			summary.Succeeded++
		}
		if before != nil && after != nil && before.State != after.State {
			switch after.State {
			case model.BalanceProtectionStateNormal:
				summary.Recovered++
			case model.BalanceProtectionStatePending,
				model.BalanceProtectionStateProtected,
				model.BalanceProtectionStateUnknown,
				model.BalanceProtectionStateInvalidAllowlist:
				summary.Protected++
			}
		} else {
			summary.Unchanged++
		}
		if report != nil {
			report(index+1, total)
		}
	}
	return summary, nil
}

func isBalanceExhaustionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"insufficient balance",
		"insufficient quota",
		"insufficient_quota",
		"quota exhausted",
		"exceeded your current quota",
		"billing hard limit",
		"balance not enough",
		"余额不足",
		"余额已用完",
		"额度不足",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
