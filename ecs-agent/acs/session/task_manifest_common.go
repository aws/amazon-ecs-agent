package session

// ManifestMessageIDAccessor stores the latest message ID that corresponds to the
// most recently processed task manifest message and is used to determine the
// validity of a task stop verification ack message.
type ManifestMessageIDAccessor interface {
	GetMessageID() string
	SetMessageID(messageID string) error
}
