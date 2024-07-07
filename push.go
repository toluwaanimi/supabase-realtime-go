package supabase_realtime_go

import (
	"time"
)

type Push struct {
	channel      *Channel
	event        string
	payload      map[string]interface{}
	timeout      time.Duration
	sent         bool
	timeoutTimer *time.Timer
	ref          string
	receivedResp *ReceivedResp
	recHooks     []RecHook
	refEvent     string
}

type RecHook struct {
	status   string
	callback func(response interface{})
}

type ReceivedResp struct {
	status   string
	response map[string]interface{}
}

func NewPush(channel *Channel, event string, payload map[string]interface{}, timeout time.Duration) *Push {
	return &Push{
		channel:  channel,
		event:    event,
		payload:  payload,
		timeout:  timeout,
		recHooks: make([]RecHook, 0),
	}
}

func (p *Push) Resend(timeout time.Duration) {
	p.timeout = timeout
	p.cancelRefEvent()
	p.ref = ""
	p.refEvent = ""
	p.receivedResp = nil
	p.sent = false
	p.Send()
}

func (p *Push) Send() {
	if p.hasReceived("timeout") {
		return
	}
	p.startTimeout()
	p.sent = true
	p.channel.socket.Push(RealtimeMessage{
		Topic:   p.channel.topic,
		Event:   p.event,
		Payload: p.payload,
		Ref:     p.ref,
		JoinRef: p.channel.joinRef(),
	})
}

func (p *Push) UpdatePayload(payload map[string]interface{}) {
	for k, v := range payload {
		p.payload[k] = v
	}
}

func (p *Push) Receive(status string, callback func(response interface{})) *Push {
	if p.hasReceived(status) {
		callback(p.receivedResp.response)
	}
	p.recHooks = append(p.recHooks, RecHook{status, callback})
	return p
}

func (p *Push) startTimeout() {
	if p.timeoutTimer != nil {
		return
	}
	p.ref = p.channel.socket.makeRef()
	p.refEvent = p.channel.replyEventName(p.ref)

	callback := func(payload interface{}) {
		p.cancelRefEvent()
		p.cancelTimeout()
		p.receivedResp = payload.(*ReceivedResp)
		p.matchReceive(p.receivedResp)
	}

	p.channel.On(p.refEvent, callback)

	p.timeoutTimer = time.AfterFunc(p.timeout, func() {
		p.trigger("timeout", map[string]interface{}{})
	})
}

func (p *Push) trigger(status string, response map[string]interface{}) {
	if p.refEvent != "" {
		p.channel.trigger(p.refEvent, &ReceivedResp{status, response})
	}
}

func (p *Push) Destroy() {
	p.cancelRefEvent()
	p.cancelTimeout()
}

func (p *Push) cancelRefEvent() {
	if p.refEvent == "" {
		return
	}
	p.channel.off(p.refEvent, "")
	p.refEvent = ""
}

func (p *Push) cancelTimeout() {
	if p.timeoutTimer != nil {
		p.timeoutTimer.Stop()
		p.timeoutTimer = nil
	}
}

func (p *Push) matchReceive(receivedResp *ReceivedResp) {
	for _, hook := range p.recHooks {
		if hook.status == receivedResp.status {
			hook.callback(receivedResp.response)
		}
	}
}

func (p *Push) hasReceived(status string) bool {
	return p.receivedResp != nil && p.receivedResp.status == status
}
