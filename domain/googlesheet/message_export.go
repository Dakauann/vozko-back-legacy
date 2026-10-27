package googlesheet

import (
	"strings"
	"time"

	"vozko/domain/conversation"
)

const (
	DirectionReceived = "recebida"
	DirectionSent     = "enviada"
	senderCustomer    = "Cliente"
	senderBusiness    = "Equipe"
	MaxExportMessages = DefaultBatchRows
)

type ExportPosition struct {
	At        time.Time
	MessageID string
}

func (p ExportPosition) IsZero() bool { return p.At.IsZero() && p.MessageID == "" }

func (p ExportPosition) Before(q ExportPosition) bool {
	if !p.At.Equal(q.At) {
		return p.At.Before(q.At)
	}
	return p.MessageID < q.MessageID
}

type ExportedMessage struct {
	ID       string
	At       time.Time
	Sender   string
	Inbound  bool
	Text     string
	MediaURL string
}

func ExportedMessageOf(m *conversation.Message) (ExportedMessage, bool) {
	if m == nil || m.IsRetracted() {
		return ExportedMessage{}, false
	}
	switch m.MessageType {
	case conversation.MessageTypeToolCall, conversation.MessageTypeToolResult, conversation.MessageTypeSystem:
		return ExportedMessage{}, false
	}
	text := strings.TrimSpace(m.ReadableText())
	mediaURL := ""
	if m.Media != nil {
		mediaURL = strings.TrimSpace(m.Media.URL)
	}
	if text == "" && mediaURL == "" {
		return ExportedMessage{}, false
	}
	inbound := m.FromCustomer()
	sender := strings.TrimSpace(m.SenderName)
	if sender == "" && inbound {
		sender = senderCustomer
	}
	if sender == "" {
		sender = senderBusiness
	}
	return ExportedMessage{ID: m.ID, At: m.CreatedAt, Sender: sender, Inbound: inbound, Text: text, MediaURL: mediaURL}, true
}

func (m ExportedMessage) Position() ExportPosition {
	return ExportPosition{At: m.At, MessageID: m.ID}
}

func (m ExportedMessage) direction() string {
	if m.Inbound {
		return DirectionReceived
	}
	return DirectionSent
}

func (s *Sheet) WithMessage(conversationValues SystemValues, m ExportedMessage) SystemValues {
	values := make(SystemValues, len(conversationValues)+5)
	for source, value := range conversationValues {
		if !source.PerMessage() {
			values[source] = value
		}
	}
	values[SourceMessageSentAt] = m.At.In(s.Location()).Format(time.RFC3339)
	values[SourceMessageSender] = m.Sender
	values[SourceMessageDirection] = m.direction()
	values[SourceMessageText] = m.Text
	values[SourceMessageMediaURL] = m.MediaURL
	return values
}

func (s *Sheet) HasMessageColumns() bool {
	for _, col := range s.Columns {
		if !col.Archived && col.Source.PerMessage() {
			return true
		}
	}
	return false
}

func KeyForMessage(entryType, entryID, messageID string) string {
	return boundKey("msg:" + entryType + ":" + entryID + ":" + messageID)
}

type MessageRow struct {
	MessageID string
	Cells     Cells
}

func NewMessageRows(s *Sheet, rows []MessageRow, origin Origin, now time.Time) ([]*Row, error) {
	out := make([]*Row, 0, len(rows))
	for i, in := range rows {
		if strings.TrimSpace(in.MessageID) == "" {
			return nil, ErrIdempotencyKeyRequired
		}
		row, err := NewRow(s, in.Cells, KeyForMessage(origin.EntryType, origin.EntryID, in.MessageID), origin, now)
		if err != nil {
			return nil, err
		}
		row.CreatedAt = now.Add(time.Duration(i) * time.Microsecond)
		out = append(out, row)
	}
	return out, nil
}

type MessageExport struct {
	WorkspaceID string
	SheetID     string
	EntryID     string
	EntryType   string
	Rows        []*Row
	Through     ExportPosition
}
