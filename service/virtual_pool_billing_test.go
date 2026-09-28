package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type virtualPoolRecordingFunding struct {
	preConsumed int
	settled     int
	refunds     int
}

func (funding *virtualPoolRecordingFunding) Source() string { return BillingSourceWallet }

func (funding *virtualPoolRecordingFunding) PreConsume(amount int) error {
	funding.preConsumed += amount
	return nil
}

func (funding *virtualPoolRecordingFunding) Settle(delta int) error {
	funding.settled += delta
	return nil
}

func (funding *virtualPoolRecordingFunding) Refund() error {
	funding.refunds += funding.preConsumed
	return nil
}

func TestVirtualPoolPartialStreamSettlementIsNotRefunded(t *testing.T) {
	funding := &virtualPoolRecordingFunding{preConsumed: 100}
	relayInfo := &relaycommon.RelayInfo{IsPlayground: true}
	session := &BillingSession{
		relayInfo:        relayInfo,
		funding:          funding,
		preConsumedQuota: 100,
		tokenConsumed:    100,
	}
	relayInfo.Billing = session
	relayInfo.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, relayInfo.AttemptOutcome.Begin("attempt-partial", "candidate"))

	require.NoError(t, session.Settle(30))
	outcome, err := relayInfo.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.True(t, outcome.UsageSettled, "settlement must be visible to controller retry/refund decisions")
	assert.Equal(t, -70, funding.settled, "settlement adjusts the pre-consumed 100 down to actual 30")

	session.Refund(nil)
	assert.Zero(t, funding.refunds, "a settled partial stream must never be fully refunded")
	assert.Equal(t, 30, funding.preConsumed+funding.settled, "net charge must remain 30 after the negative settlement delta")
}
