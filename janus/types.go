package janus

import (
	"encoding/json"
	"fmt"
)

const (
	SDPOffer  = "offer"
	SDPAnswer = "answer"
)

type IceCandidate struct {
	SdpMid        string `json:"sdpMid"`
	SdpMLineIndex int    `json:"sdpMLineIndex"`
	Candidate     string `json:"candidate"`
}

// GeneralMsg is base message format which only contain fields needed to further parse message to more specific type
// and process them such as complete the request transaction and pass to session or plugin handle event channel
type GeneralMsg struct {
	Janus       string `json:"janus"`
	Transaction string `json:"transaction"`
	SessionID   int64  `json:"session_id"`
	Sender      int64  `json:"sender"` // janus plugin handle id
}

type AckMsg GeneralMsg

type ErrorMsg struct {
	Err struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	} `json:"error"`
}

func (e *ErrorMsg) Error() string {
	return fmt.Sprintf("err code(%d): %s", e.Err.Code, e.Err.Reason)
}

type ServerInfoMsg struct {
	Name         string                       `json:"name"`
	Version      int                          `json:"version"`
	VersionStr   string                       `json:"version_string"`
	Author       string                       `json:"author"`
	DataChannels bool                         `json:"data_channels"`
	IPV6         bool                         `json:"ipv_6"`
	Transports   map[string]TransportInfoData `json:"transports"`
	Plugins      map[string]PluginInfoData    `json:"plugins"`
}

type TransportInfoData struct {
	Name          string `json:"name"`
	Author        string `json:"author"`
	Description   string `json:"description"`
	VersionString string `json:"version_string"`
	Version       int    `json:"version"`
}

type PluginInfoData struct {
	VersionString string `json:"version_string"`
	Description   string `json:"description"`
	Author        string `json:"author"`
	Name          string `json:"name"`
	Version       int    `json:"version"`
}

// SuccessMsg
// create session:
// {
//   "janus": "success",
//   "transaction": "25iarrmd8JtOvIetLmgkdWkqfvi",
//   "data": {
//      "id": 6263157895990541
//   }
// }
//
// attach plugin:
// {
//   "janus": "success",
//   "session_id": 6263157895990541,
//   "transaction": "25iaru1uqWQHTSXnuztTs11X74B",
//   "data": {
//      "id": 7232873872272105
//   }
// }
//
// synchronous plugin requests response:
// {
//   "janus": "success",
//   "session_id": 6263157895990541,
//   "transaction": "25iarrkbpoC7zFi7jtWfe4AndqK",
//   "sender": 7232873872272105,
//   "plugindata": {
//      "plugin": "janus.plugin.audiobridge",
//      "data": {
//         "audiobridge": "created",
//         "room": 3759084404994365,
//         "permanent": false
//      }
//   }
// }
type SuccessMsg struct {
	Janus       string      `json:"janus"`
	Transaction string      `json:"transaction"`
	SessionID   int64       `json:"session_id"`
	Sender      int64       `json:"sender"`
	PluginData  PluginData  `json:"plugindata"`
	Data        SuccessData `json:"data"`
}

type SuccessData struct {
	ID int64 `json:"id"`
}

// EventMsg
// note: event message may have transaction if this is an async request, or empty for message
// that is solely come from plugin
type EventMsg struct {
	Janus       string     `json:"janus"`
	Transaction string     `json:"transaction"`
	SessionID   int64      `json:"session_id"`
	Sender      int64      `json:"sender"`
	PluginData  PluginData `json:"plugindata"`
	JSEP        *JSEPData  `json:"jsep"`
}

type PluginData struct {
	Plugin string          `json:"plugin"`
	Data   json.RawMessage `json:"data"` // contains plugin specific payload
}

type JSEPData struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

type WebRTCUpMsg GeneralMsg

type MediaMsg struct {
	Janus       string `json:"janus"`
	Transaction string `json:"transaction"`
	SessionID   int64  `json:"session_id"`
	Sender      int64  `json:"sender"` // janus plugin handle id
	Type        string `json:"type"`   // audio|video
	Receiving   bool   `json:"receiving"`
	Mid         string `json:"mid"`
}

type SlowLinkMsg struct {
	Janus       string `json:"janus"`
	Transaction string `json:"transaction"`
	SessionID   int64  `json:"session_id"`
	Sender      int64  `json:"sender"` // janus plugin handle id
	Uplink      bool   `json:"uplink"`
	Nacks       int64  `json:"nacks"`
}

type HangupMsg struct {
	Janus       string `json:"janus"`
	Transaction string `json:"transaction"`
	SessionID   int64  `json:"session_id"`
	Sender      int64  `json:"sender"` // janus plugin handle id
	Reason      string `json:"reason"`
}

type DetachedMsg struct {
	Janus     string `json:"janus"`
	SessionID int64  `json:"session_id"`
	Sender    int64  `json:"sender"`
}

type TimeoutMsg struct {
	Janus     string `json:"janus"`
	SessionID int64  `json:"session_id"`
}

var msgTypesMapper = map[string]func() interface{}{
	"error":       func() interface{} { return &ErrorMsg{} },
	"server_info": func() interface{} { return &ServerInfoMsg{} },
	"success":     func() interface{} { return &SuccessMsg{} },
	"ack":         func() interface{} { return &AckMsg{} },
	"event":       func() interface{} { return &EventMsg{} },
	"webrtcup":    func() interface{} { return &WebRTCUpMsg{} },
	"media":       func() interface{} { return &MediaMsg{} },
	"slowlink":    func() interface{} { return &SlowLinkMsg{} },
	"hangup":      func() interface{} { return &HangupMsg{} },
	"detached":    func() interface{} { return &DetachedMsg{} },
	"timeout":     func() interface{} { return &TimeoutMsg{} },
}
