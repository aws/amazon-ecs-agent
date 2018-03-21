package wsconn

import "time"

// WebsocketConn specifies the subset of gorilla/websocket's
// connection's methods that this client uses.
type WebsocketConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, data []byte, err error)
	Close() error
	SetWriteDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
}

//go:generate go run ../../../scripts/generate/mockgen.go github.com/aws/amazon-ecs-agent/agent/wsclient/wsconn WebsocketConn mock/$GOFILE
