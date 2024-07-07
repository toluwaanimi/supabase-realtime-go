package supabase_realtime_go

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"time"
)

type Channel struct {
	topic                string
	params               RealtimeChannelOptions
	socket               *Client
	bindings             map[string][]Binding
	timeout              time.Duration
	state                ChannelState
	joinedOnce           bool
	joinPush             *Push
	rejoinTimer          *Timer
	pushBuffer           []*Push
	presence             *RealtimePresence
	broadcastEndpointURL string
	subTopic             string
	private              bool
}

type Binding struct {
	Type     string
	Filter   map[string]interface{}
	Callback func(payload interface{})
	ID       string
}

func NewChannel(topic string, params RealtimeChannelOptions, socket *Client) *Channel {
	channel := &Channel{
		topic:      topic,
		params:     params,
		socket:     socket,
		state:      CHANNEL_STATE_CLOSED,
		bindings:   make(map[string][]Binding),
		pushBuffer: make([]*Push, 0),
	}

	channel.joinPush = NewPush(channel, CHANNEL_EVENT_JOIN, nil, time.Duration(channel.socket.timeout)*time.Millisecond)
	channel.rejoinTimer = NewTimer(channel.socket.reconnectAfterMs, channel.rejoinUntilConnected)
	channel.subTopic = topic
	channel.timeout = time.Duration(channel.socket.timeout) * time.Millisecond

	channel.joinPush.Receive("ok", func(response interface{}) {
		channel.state = CHANNEL_STATE_JOINED
		channel.rejoinTimer.Reset()
		for _, pushEvent := range channel.pushBuffer {
			pushEvent.Send()
		}
		channel.pushBuffer = []*Push{}
	})

	channel.On(CHANNEL_EVENT_CLOSE, nil, func(payload interface{}) { channel.onClose(payload) })
	channel.On(CHANNEL_EVENT_ERROR, nil, func(payload interface{}) { channel.onError(payload) })
	channel.On(CHANNEL_EVENT_REPLY, nil, func(payload interface{}) { channel.onReply(payload) })

	return channel
}

func (c *Channel) replyEventName(ref string) string {
	return fmt.Sprintf("chan_reply_%s", ref)
}

func (c *Channel) Subscribe(callback func(status string, err error), timeout time.Duration) error {
	if c.joinedOnce {
		return errors.New("tried to subscribe multiple times. 'subscribe' can only be called a single time per channel instance")
	}

	c.joinedOnce = true
	c.rejoin(timeout)

	c.joinPush.Receive("ok", func(response interface{}) {
		callback("SUBSCRIBED", nil)
	}).Receive("error", func(response interface{}) {
		callback("CHANNEL_ERROR", fmt.Errorf("%v", response))
	}).Receive("timeout", func(response interface{}) {
		callback("TIMED_OUT", nil)
	})

	return nil
}

func (c *Channel) rejoinUntilConnected() {
	c.rejoinTimer.ScheduleTimeout()
	if c.socket.IsConnected() {
		c.rejoin(c.timeout)
	}
}

func (c *Channel) rejoin(timeout time.Duration) {
	if c.state == CHANNEL_STATE_LEAVING {
		return
	}
	c.socket.leaveOpenTopic(c.topic)
	c.state = CHANNEL_STATE_JOINING
	c.joinPush.Resend(timeout)
}

func (c *Channel) onClose(payload interface{}) {
	c.rejoinTimer.Reset()
	c.state = CHANNEL_STATE_CLOSED
	c.socket.removeChannel(c)
}

func (c *Channel) onError(payload interface{}) {
	if c.state == CHANNEL_STATE_LEAVING || c.state == CHANNEL_STATE_CLOSED {
		return
	}
	c.state = CHANNEL_STATE_ERRORED
	c.rejoinTimer.ScheduleTimeout()
}

func (c *Channel) onReply(payload interface{}) {
	c.trigger(fmt.Sprintf("chan_reply_%s", payload.(map[string]interface{})["ref"].(string)), payload)
}

func (c *Channel) trigger(event string, payload interface{}) {
	bindings, ok := c.bindings[event]
	if ok {
		for _, binding := range bindings {
			binding.Callback(payload)
		}
	}
}

func (c *Channel) On(eventType string, filter map[string]interface{}, callback func(payload interface{})) {
	binding := Binding{
		Type:     eventType,
		Filter:   filter,
		Callback: callback,
		ID:       uuid.New().String(),
	}
	c.bindings[eventType] = append(c.bindings[eventType], binding)
}

func (c *Channel) off(event string, callbackID string) {
	bindings := c.bindings[event]
	for i, binding := range bindings {
		if binding.ID == callbackID {
			c.bindings[event] = append(c.bindings[event], bindings[i+1:]...)
			break
		}
	}
}

func (c *Channel) joinRef() string {
	return c.joinPush.ref
}

func (c *Channel) Send(args map[string]interface{}, opts map[string]interface{}) (string, error) {
	if !c.canPush() && args["type"] == REALTIME_LISTEN_TYPE_BROADCAST {
		options := map[string]interface{}{
			"method": "POST",
			"headers": map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", c.socket.accessToken),
				"apikey":        c.socket.apiKey,
				"Content-Type":  "application/json",
			},
			"body": map[string]interface{}{
				"messages": []map[string]interface{}{
					{
						"topic":   c.subTopic,
						"event":   args["event"],
						"payload": args["payload"],
						"private": c.private,
					},
				},
			},
		}

		// Serialize the request body to JSON
		bodyBytes, err := json.Marshal(options["body"])
		if err != nil {
			return "error", err
		}

		// Create a new HTTP request
		req, err := http.NewRequest("POST", c.broadcastEndpointURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return "error", err
		}

		// Set headers
		for key, value := range options["headers"].(map[string]string) {
			req.Header.Set(key, value)
		}

		// Create an HTTP client and perform the request
		client := &http.Client{
			Timeout: time.Duration(c.socket.timeout) * time.Millisecond,
		}
		resp, err := client.Do(req)
		if err != nil {
			return "error", err
		}
		defer resp.Body.Close()

		// Check the response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return "ok", nil
		} else if resp.StatusCode == http.StatusRequestTimeout {
			return "timed out", nil
		} else {
			return "error", fmt.Errorf("HTTP error: %s", resp.Status)
		}
	} else {
		// Create a channel to receive the result from the callbacks
		resultChan := make(chan string, 1)

		push := c.push(args["type"].(string), args, c.timeout)
		if args["type"] == REALTIME_LISTEN_TYPE_BROADCAST && !c.params.Config.Broadcast.Ack {
			return "ok", nil
		}

		push.Receive("ok", func(response interface{}) {
			resultChan <- "ok"
		}).Receive("error", func(response interface{}) {
			resultChan <- "error"
		}).Receive("timeout", func(response interface{}) {
			resultChan <- "timed out"
		})

		// Wait for the result and return it
		return <-resultChan, nil
	}
}

func (c *Channel) canPush() bool {
	return c.socket.IsConnected() && c.state == CHANNEL_STATE_JOINED
}

func (c *Channel) push(event string, payload map[string]interface{}, timeout time.Duration) *Push {
	if !c.joinedOnce {
		panic(fmt.Sprintf("tried to push '%s' to '%s' before joining. Use channel.subscribe() before pushing events", event, c.topic))
	}
	push := NewPush(c, event, payload, timeout)
	if c.canPush() {
		push.Send()
	} else {
		push.startTimeout()
		c.pushBuffer = append(c.pushBuffer, push)
	}
	return push
}

func (c *Channel) UpdateJoinPayload(payload map[string]interface{}) {
	c.joinPush.UpdatePayload(payload)
}

func (c *Channel) Unsubscribe(timeout time.Duration) error {
	c.state = CHANNEL_STATE_LEAVING
	onClose := func() {
		c.socket.log("channel", fmt.Sprintf("leave %s", c.topic), nil)
		c.trigger(CHANNEL_EVENT_CLOSE, map[string]interface{}{"leave": c.joinRef()})
	}

	c.rejoinTimer.Reset()
	c.joinPush.Destroy()

	push := NewPush(c, CHANNEL_EVENT_LEAVE, nil, timeout)

	var errChan = make(chan error, 1)

	push.Receive("ok", func(response interface{}) {
		onClose()
		errChan <- nil
	}).Receive("timeout", func(response interface{}) {
		onClose()
		errChan <- errors.New("timeout")
	}).Receive("error", func(response interface{}) {
		errChan <- errors.New("error")
	})

	push.Send()
	return <-errChan
}
