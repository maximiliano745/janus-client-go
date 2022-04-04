package janus

import (
	"github.com/rs/zerolog"
)

// Handle represent instance to plugin within a session
// ref: https://janus.conf.meetecho.com/docs/rest.html#handles
type Handle struct {
	id         int64
	sessionID  int64
	events     *chanBroadcast
	detachHook func(handleID int64)
	gateway    *Gateway
	isValid    bool
	logger     zerolog.Logger
}

func (h *Handle) passMsg(msg interface{}) {
	h.events.publish(msg)
}

func (h *Handle) IsValid() bool {
	return h.isValid
}

func (h *Handle) ID() int64 {
	return h.id
}

func (h *Handle) SessionID() int64 {
	return h.sessionID
}

func (h *Handle) Events() <-chan interface{} {
	return h.events.subscribe()
}

// Request send synchronous plugin specific request to janus server
func (h *Handle) Request(body map[string]interface{}) (*SuccessMsg, error) {
	msg := map[string]interface{}{
		"janus":      "message",
		"session_id": h.sessionID,
		"handle_id":  h.id,
		"body":       body,
	}

	resp, err := h.gateway.sendRequestSync(msg)
	if err != nil {
		return nil, err
	}

	switch resp := resp.(type) {
	case *SuccessMsg:
		return resp, nil
	case *ErrorMsg:
		return nil, resp
	default:
		return nil, ErrUnexpected
	}
}

// Message sends asynchronous plugin specific message
// ref: https://janus.conf.meetecho.com/docs/rest.html#handles
// plugin can response to the message synchronously or asynchronously
// which means we can receive 'ack' or 'success' message
// 'ack' means plugin already received the message and will process it soon and will emit 'event'
// 'success' may means it successfully processed
func (h *Handle) Message(body map[string]interface{}, jsep *JSEPData) (*EventMsg, error) {
	msg := map[string]interface{}{
		"janus":      "message",
		"session_id": h.sessionID,
		"handle_id":  h.id,
		"body":       body,
	}

	if jsep != nil {
		msg["jsep"] = jsep
	}

	resp, err := h.gateway.sendMessageSync(msg)
	if err != nil {
		return nil, err
	}

	switch resp := resp.(type) {
	case *AckMsg:
		return nil, nil
	case *EventMsg:
		return resp, nil
	case *ErrorMsg:
		return nil, resp
	default:
		return nil, ErrUnexpected
	}
}

// Trickle sends single or multiple trickled ice candidate or complete sign
func (h *Handle) Trickle(candidates ...IceCandidate) error {
	body := map[string]interface{}{
		"janus":      "trickle",
		"session_id": h.sessionID,
		"handle_id":  h.id,
	}

	if len(candidates) <= 0 {
		body["candidate"] = map[string]interface{}{
			"complete": true,
		}
	} else if len(candidates) == 1 {
		body["candidate"] = candidates[0]
	} else {
		body["candidates"] = candidates
	}

	resp, err := h.gateway.sendRequestSync(body)
	if err != nil {
		return err
	}

	switch resp := resp.(type) {
	case *AckMsg:
		h.logger.Debug().Msg("send trickle ok")
		return nil
	case *ErrorMsg:
		return resp
	default:
		return ErrUnexpected
	}
}

// HangUp close currently ongoing webrtc connection without destroying plugin handle
func (h *Handle) HangUp() error {
	body := map[string]interface{}{
		"janus":      "hangup",
		"session_id": h.sessionID,
		"handle_id":  h.id,
	}

	resp, err := h.gateway.sendRequestSync(body)
	if err != nil {
		return err
	}

	switch resp := resp.(type) {
	case *SuccessMsg:
		return nil
	case *ErrorMsg:
		return resp
	default:
		return ErrUnexpected
	}
}

// Detach destroy current plugin handle
func (h *Handle) Detach() error {
	body := map[string]interface{}{
		"janus":      "detach",
		"session_id": h.sessionID,
		"handle_id":  h.id,
	}

	resp, err := h.gateway.sendRequestSync(body)
	if err != nil {
		return err
	}

	switch resp := resp.(type) {
	case *SuccessMsg:
		h.isValid = false
		h.detachHook(h.id)
		return nil
	case *ErrorMsg:
		return resp
	default:
		return ErrUnexpected
	}
}
