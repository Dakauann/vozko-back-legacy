package businessphone

import "time"

type AccountAccessRecorder interface {
	RecordAccountAccess(phoneID string, lostAt *time.Time) (bool, error)
}

func (p *WhatsAppBusinessPhoneNumber) NeedsReconnect() bool {
	return p.AccountAccessLostAt != nil
}

func (p *WhatsAppBusinessPhoneNumber) Reconnected() {
	p.AccountAccessLostAt = nil
}
