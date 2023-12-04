package videoroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/maximiliano745/janus-client-go/janus"
	"github.com/rs/zerolog/log"
)

type StreamInfoReq struct {
	Feed int64
	Mid  string
}

func (s StreamInfoReq) toJanusObjMap() map[string]interface{} {
	janusStream := map[string]interface{}{"feed": s.Feed}
	if s.Mid != "" {
		janusStream["mid"] = s.Mid
	}
	return janusStream
}

type UnsubscribeStreamInfoReq struct {
	Feed   int64
	Mid    string
	SubMid string
}

func (s UnsubscribeStreamInfoReq) toJanusObjMap() map[string]interface{} {
	janusStream := map[string]interface{}{"feed": s.Feed}
	if s.Mid != "" {
		janusStream["mid"] = s.Mid
	}
	if s.SubMid != "" {
		janusStream["sub_mid"] = s.SubMid
	}
	return janusStream
}

type StreamInfoResp struct {
	Mindex      int    `json:"mindex"`
	Mid         string `json:"mid"`
	Type        string `json:"type"`
	FeedID      int64  `json:"feed_id"`
	FeedMid     string `json:"feed_mid"`
	FeedDisplay string `json:"feed_display"`
	Send        bool   `json:"send"`
	Ready       bool   `json:"ready"`
}

func (s StreamInfoResp) IsValid() bool {
	return s.FeedID > 0 && s.FeedMid != ""
}

type UpdatedEventMsg struct {
	Room    int64 `json:"room"`
	Streams []struct {
		Type    string `json:"type"`
		Active  bool   `json:"active"`
		Mindex  int    `json:"mindex"`
		Mid     string `json:"mid"`
		Ready   bool   `json:"ready"`
		Send    bool   `json:"send"`
		FeedID  int64  `json:"feed_id"`
		FeedMid string `json:"feed_mid"`
		Codec   string `json:"codec"`
	} `json:"streams"`
	JSEP *janus.JSEPData `json:"-"`
}

type Subscriber struct {
	ctx    context.Context
	handle *janus.Handle
	roomID int64

	// OnUpdatedEvent called when renegotiation is needed,
	// such as when publishers webrtc connection gone (disconnected)
	OnUpdatedEvent func(msg UpdatedEventMsg)
}

func NewSubscriber(ctx context.Context, handle *janus.Handle, roomID int64) *Subscriber {
	s := &Subscriber{
		ctx:    ctx,
		handle: handle,
		roomID: roomID,
	}
	go s.eventLoop()
	return s
}

func (s *Subscriber) Handle() *janus.Handle {
	return s.handle
}

// Join subscribe to specified streams (multistream)
// return sdp offer if success
func (s *Subscriber) Join(streams ...StreamInfoReq) ([]StreamInfoResp, string, error) {
	req := map[string]interface{}{
		"request": "join",
		"ptype":   TypeSubscriber,
		"room":    s.roomID,
	}

	if len(streams) > 0 {
		var streamsAttr []map[string]interface{}
		for _, stream := range streams {
			streamsAttr = append(streamsAttr, stream.toJanusObjMap())
		}
		req["streams"] = streamsAttr
	}

	resp, err := s.handle.Message(req, nil)
	if err != nil {
		return nil, "", err
	}

	pluginData := struct {
		VideoRoom string           `json:"videoroom"`
		Room      int64            `json:"room"`
		Streams   []StreamInfoResp `json:"streams"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return nil, "", err
	}

	if pluginData.VideoRoom != "attached" {
		log.Debug().Msg("videoroom:join (subscriber) unexpected response event")
		return nil, "", fmt.Errorf("unexpected response: videoroom %s", pluginData.VideoRoom)
	}

	if resp.JSEP == nil {
		log.Debug().Msg("videoroom:join (subscriber) jsep nil")
		return nil, "", fmt.Errorf("unexpected response: jsep nil")
	}

	return pluginData.Streams, resp.JSEP.Sdp, nil
}

func (s *Subscriber) Start(sdpAnswer string) error {
	req := map[string]interface{}{
		"request": "start",
	}
	resp, err := s.handle.Message(req, &janus.JSEPData{
		Type: janus.SDPAnswer,
		Sdp:  sdpAnswer,
	})
	if err != nil {
		return err
	}

	pluginData := RawVideoRoomEventMsg{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return err
	}

	if pluginData.GetString("started") != "ok" {
		log.Debug().Msg("videoroom:join (subscriber) started not ok")
		return errors.New("unexpected response: started != ok")
	}
	return nil
}

func (p *Publisher) Pause() error {
	req := map[string]interface{}{
		"request": "pause",
	}

	resp, err := p.handle.Message(req, nil)
	if err != nil {
		return err
	}

	pluginData := RawVideoRoomEventMsg{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return err
	}

	if pluginData.GetString("paused") != "ok" {
		return errors.New("unexpected response: paused != ok")
	}
	return nil
}

// Subscribe to streams owned by publishers
// note: need to avoid duplicate subscription to already subscribed publisher
func (s *Subscriber) Subscribe(streams ...StreamInfoReq) ([]StreamInfoResp, string, error) {
	req := map[string]interface{}{
		"request": "subscribe",
	}

	var janusStreamList []map[string]interface{}
	for _, stream := range streams {
		janusStreamList = append(janusStreamList, stream.toJanusObjMap())
	}
	req["streams"] = janusStreamList

	resp, err := s.handle.Message(req, nil)
	if err != nil {
		return nil, "", err
	}

	pluginData := struct {
		VideoRoom string           `json:"videoroom"`
		Room      int64            `json:"room"`
		Streams   []StreamInfoResp `json:"streams"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return nil, "", err
	}

	if pluginData.VideoRoom != "updated" {
		log.Debug().Msg("videoroom:subscribe (subscriber) video room != updated")
		return nil, "", fmt.Errorf("unexpected response: video room != updated")
	}

	if resp.JSEP != nil {
		return pluginData.Streams, resp.JSEP.Sdp, nil
	}

	return nil, "", nil
}

func (s *Subscriber) Unsubscribe(streams ...UnsubscribeStreamInfoReq) ([]StreamInfoResp, string, error) {
	req := map[string]interface{}{
		"request": "unsubscribe",
	}

	var janusStreamList []map[string]interface{}
	for _, stream := range streams {
		janusStreamList = append(janusStreamList, stream.toJanusObjMap())
	}
	req["streams"] = janusStreamList

	resp, err := s.handle.Message(req, nil)
	if err != nil {
		return nil, "", err
	}

	pluginData := struct {
		VideoRoom string           `json:"videoroom"`
		Room      int64            `json:"room"`
		Streams   []StreamInfoResp `json:"streams"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return nil, "", err
	}

	if pluginData.VideoRoom != "updated" {
		log.Debug().Msg("videoroom:unsubscribe (subscriber) video room != updated")
		return nil, "", fmt.Errorf("unexpected response: video room != updated")
	}

	if resp.JSEP != nil {
		return pluginData.Streams, resp.JSEP.Sdp, nil
	}

	return pluginData.Streams, "", nil
}

func (s *Subscriber) Leave() error {
	req := map[string]interface{}{
		"request": "leave",
	}

	// expect response left event
	// {
	//   "videoroom" : "event",
	//   "left" : "ok",
	// }
	_, err := s.handle.Message(req, nil)
	return err
}

func (s *Subscriber) eventLoop() {
	eventChan := s.handle.Events()
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt := <-eventChan:
			switch msg := evt.(type) {
			case *janus.EventMsg:
				videoRoomMsg := RawVideoRoomEventMsg{}
				_ = json.Unmarshal(msg.PluginData.Data, &videoRoomMsg)

				if videoRoomMsg.IsVideoRoomEvent("updated") {
					// kalau "updated" event ini memiliki jsep maka itu harus diterima client untuk direnegotiate
					// dan mengirimkan kembali sdp answer ke janus didalam request "start"
					// jika tidak maka statenya akan menggantung, tidak bisa "subscribe"/"unsubscribe" (request lain
					// yang berhubungan dengan negosiasi sdp)
					updatedMsg := UpdatedEventMsg{}
					_ = json.Unmarshal(msg.PluginData.Data, &updatedMsg)
					updatedMsg.JSEP = msg.JSEP
					if s.OnUpdatedEvent != nil {
						s.OnUpdatedEvent(updatedMsg)
					}
					continue
				}
			}
		}
	}
}
