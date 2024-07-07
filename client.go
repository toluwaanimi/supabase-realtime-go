package supabase_realtime_go

import (
	"fmt"
	"sync"
	"time"
)

type Client struct {
	accessToken          string
	apiKey               string
	channels             []*Channel
	endPoint             string
	headers              map[string]string
	params               map[string]string
	timeout              int
	heartbeatIntervalMs  int
	heartbeatTimer       *time.Timer
	pendingHeartbeatRef  string
	ref                  int
	reconnectTimer       *Timer
	logger               func(kind, msg string, data interface{})
	encode               func(payload interface{}, callback func(result string))
	decode               func(rawMessage []byte, callback func(msg RealtimeMessage))
	reconnectAfterMs     func(tries int) time.Duration
	conn                 WebSocketLike
	sendBuffer           []func()
	serializer           *Serializer
	stateChangeCallbacks StateChangeCallbacks
	fetch                func(url string, options map[string]interface{}) (Response, error)
	mutex                sync.Mutex
}

type RealtimeMessage struct {
	Topic   string
	Event   string
	Payload map[string]interface{}
	Ref     string
	JoinRef string
}

type StateChangeCallbacks struct {
	Open    []func()
	Close   []func()
	Error   []func(error)
	Message []func(msg RealtimeMessage)
}

type Response struct {
	Status int
	Body   []byte
}

func NewClient(endPoint string, options ...func(*Client)) *Client {
	c := &Client{
		endPoint:            endPoint,
		headers:             make(map[string]string),
		params:              make(map[string]string),
		timeout:             DEFAULT_TIMEOUT,
		heartbeatIntervalMs: 30000,
		reconnectAfterMs: func(tries int) time.Duration {
			switch tries {
			case 1:
				return 1 * time.Second
			case 2:
				return 2 * time.Second
			case 3:
				return 5 * time.Second
			default:
				return 10 * time.Second
			}
		},
		stateChangeCallbacks: StateChangeCallbacks{
			Open:    []func(){},
			Close:   []func(){},
			Error:   []func(error){},
			Message: []func(RealtimeMessage){},
		},
	}

	for _, option := range options {
		option(c)
	}

	c.reconnectTimer = NewTimer(c.reconnectAfterMs, c.connect)
	c.serializer = NewSerializer()

	return c
}

func (c *Client) connect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil && c.conn.IsConnected() {
		return
	}

	c.conn = NewWebSocketConn(c.endPoint)
	c.conn.SetHandlers(c.onConnOpen, c.onConnClose, c.onConnError, c.onConnMessage)

	err := c.conn.Connect()
	if err != nil {
		c.onConnError(err)
	}
}

func (c *Client) disconnect(code int, reason string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil {
		c.conn.Close(code, reason)
		c.conn = nil
	}

	if c.heartbeatTimer != nil {
		c.heartbeatTimer.Stop()
	}

	c.reconnectTimer.Reset()
}

func (c *Client) onConnOpen() {
	c.logger("transport", "connected to "+c.endPoint, nil)
	c.flushSendBuffer()
	c.reconnectTimer.Reset()

	c.heartbeatTimer = time.AfterFunc(time.Duration(c.heartbeatIntervalMs)*time.Millisecond, c.sendHeartbeat)

	for _, callback := range c.stateChangeCallbacks.Open {
		callback()
	}
}

func (c *Client) onConnClose() {
	c.logger("transport", "close", nil)
	c.triggerChanError()

	if c.heartbeatTimer != nil {
		c.heartbeatTimer.Stop()
	}

	c.reconnectTimer.ScheduleTimeout()

	for _, callback := range c.stateChangeCallbacks.Close {
		callback()
	}
}

func (c *Client) onConnError(error error) {
	c.logger("transport", error.Error(), nil)
	c.triggerChanError()

	for _, callback := range c.stateChangeCallbacks.Error {
		callback(error)
	}
}

func (c *Client) onConnMessage(rawMessage []byte) {
	c.decode(rawMessage, func(msg RealtimeMessage) {
		if msg.Ref == c.pendingHeartbeatRef {
			c.pendingHeartbeatRef = ""
		}

		c.logger("receive", msg.Topic+" "+msg.Event+" "+msg.Ref, msg.Payload)

		for _, channel := range c.channels {
			if channel.topic == msg.Topic {
				channel.trigger(msg.Event, msg.Payload)
			}
		}

		for _, callback := range c.stateChangeCallbacks.Message {
			callback(msg)
		}
	})
}

func (c *Client) Push(data RealtimeMessage) {
	c.logger("push", data.Topic+" "+data.Event+" "+data.Ref, data.Payload)
	if c.conn.IsConnected() {
		c.encode(data, func(result string) {
			c.conn.Send([]byte(result))
		})
	} else {
		c.sendBuffer = append(c.sendBuffer, func() {
			c.Push(data)
		})
	}
}

func (c *Client) SetAuth(token string) {
	c.accessToken = token

	for _, channel := range c.channels {
		channel.UpdateJoinPayload(map[string]interface{}{"access_token": token})

		if channel.state == CHANNEL_STATE_JOINED {
			channel.push(CHANNEL_EVENT_ACCESS_TOKEN, map[string]interface{}{"access_token": token}, channel.timeout)
		}
	}
}

func (c *Client) makeRef() string {
	c.ref++
	return fmt.Sprintf("%d", c.ref)
}

func (c *Client) flushSendBuffer() {
	if c.conn.IsConnected() && len(c.sendBuffer) > 0 {
		for _, callback := range c.sendBuffer {
			callback()
		}
		c.sendBuffer = nil
	}
}

func (c *Client) sendHeartbeat() {
	if c.conn.IsConnected() {
		if c.pendingHeartbeatRef != "" {
			c.pendingHeartbeatRef = ""
			c.logger("transport", "heartbeat timeout. Attempting to re-establish connection", nil)
			c.conn.Close(WS_CLOSE_NORMAL, "heartbeat timeout")
			return
		}

		c.pendingHeartbeatRef = c.makeRef()
		c.Push(RealtimeMessage{
			Topic:   "phoenix",
			Event:   "heartbeat",
			Payload: nil,
			Ref:     c.pendingHeartbeatRef,
		})
		c.SetAuth(c.accessToken)
	}
}

func (c *Client) triggerChanError() {
	for _, channel := range c.channels {
		channel.trigger(CHANNEL_EVENT_ERROR, nil)
	}
}

func (c *Client) log(kind, msg string, data interface{}) {
	if c.logger != nil {
		c.logger(kind, msg, data)
	}
}

func (c *Client) leaveOpenTopic(topic string) {
	for _, channel := range c.channels {
		if channel.topic == topic && (channel.state == CHANNEL_STATE_JOINED || channel.state == CHANNEL_STATE_JOINING) {
			c.log("transport", fmt.Sprintf("leaving duplicate topic \"%s\"", topic), nil)
			channel.Unsubscribe(time.Duration(c.timeout) * time.Millisecond)
		}
	}
}

func (c *Client) removeChannel(channel *Channel) {
	for i, ch := range c.channels {
		if ch == channel {
			c.channels = append(c.channels[:i], c.channels[i+1:]...)
			break
		}
	}
}

func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

func (c *Client) Channel(topic string, params RealtimeChannelOptions) *Channel {
	channel := NewChannel(topic, params, c)
	c.channels = append(c.channels, channel)
	return channel
}
