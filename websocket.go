package supabase_realtime_go

import (
	"github.com/gorilla/websocket"
	"net/url"
)

type WebSocketConn struct {
	conn      *websocket.Conn
	endPoint  string
	onOpen    func()
	onClose   func()
	onError   func(error)
	onMessage func([]byte)
}

func NewWebSocketConn(endPoint string) *WebSocketConn {
	return &WebSocketConn{
		endPoint: endPoint,
	}
}

func (ws *WebSocketConn) Connect() error {
	u, err := url.Parse(ws.endPoint)
	if err != nil {
		return err
	}
	u.Scheme = "ws"
	ws.conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	ws.onOpen()

	go func() {
		defer ws.conn.Close()
		for {
			_, message, err := ws.conn.ReadMessage()
			if err != nil {
				ws.onError(err)
				break
			}
			ws.onMessage(message)
		}
		ws.onClose()
	}()

	return nil
}

func (ws *WebSocketConn) Close(code int, reason string) error {
	return ws.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
}

func (ws *WebSocketConn) Send(data []byte) error {
	return ws.conn.WriteMessage(websocket.TextMessage, data)
}

func (ws *WebSocketConn) IsConnected() bool {
	return ws.conn != nil && ws.conn.UnderlyingConn() != nil
}

func (ws *WebSocketConn) SetHandlers(onOpen, onClose func(), onError func(error), onMessage func([]byte)) {
	ws.onOpen = onOpen
	ws.onClose = onClose
	ws.onError = onError
	ws.onMessage = onMessage
}
