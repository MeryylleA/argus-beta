package channel

import (
	"context"
	"fmt"
	"time"

	"github.com/argus-beta/argus/internal/memory"
)

// ChannelMessage is a simplified view of a channel message for consumers.
type ChannelMessage struct {
	From    string
	MsgType string // "finding" | "question" | "context" | "duplicate"
	Content string
	SentAt  time.Time
}

// Channel provides inter-agent communication for collaborative mode.
// It is a thin wrapper around memory.Store's channel methods.
type Channel struct {
	store     memory.Store
	sessionID string
}

// New creates a channel bound to a specific session.
func New(store memory.Store, sessionID string) *Channel {
	return &Channel{
		store:     store,
		sessionID: sessionID,
	}
}

// Post sends a message from one agent to the other.
func (c *Channel) Post(ctx context.Context, from, to, msgType, content string) error {
	msg := &memory.ChannelMessage{
		SessionID: c.sessionID,
		FromAgent: from,
		ToAgent:   to,
		MsgType:   msgType,
		Content:   content,
	}
	return c.store.PostMessage(ctx, msg)
}

// Poll returns unread messages for an agent and marks them as read.
func (c *Channel) Poll(ctx context.Context, toAgent string) ([]ChannelMessage, error) {
	msgs, err := c.store.PollMessages(ctx, c.sessionID, toAgent)
	if err != nil {
		return nil, err
	}

	if len(msgs) == 0 {
		return nil, nil
	}

	// Mark as read
	if err := c.store.MarkMessagesRead(ctx, c.sessionID, toAgent); err != nil {
		return nil, fmt.Errorf("channel: failed to mark messages read: %w", err)
	}

	result := make([]ChannelMessage, len(msgs))
	for i, m := range msgs {
		result[i] = ChannelMessage{
			From:    m.FromAgent,
			MsgType: m.MsgType,
			Content: m.Content,
			SentAt:  m.CreatedAt,
		}
	}
	return result, nil
}

// FormatMessages returns a human-readable string of channel messages.
// Used to inject channel contents into the agent's tool result.
func FormatMessages(msgs []ChannelMessage) string {
	if len(msgs) == 0 {
		return "No new messages from your partner."
	}
	var b []byte
	for _, m := range msgs {
		b = append(b, fmt.Sprintf("[%s] %s (%s): %s\n",
			m.SentAt.Format("15:04:05"), m.From, m.MsgType, m.Content)...)
	}
	return string(b)
}
