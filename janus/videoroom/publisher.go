package videoroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abdularis/janus-client-go/janus"
	"github.com/rs/zerolog/log"
)

// PublisherResp returned when "joined", "event" (with publishers field)
type PublisherResp struct {
	ID      int64  `json:"id"`
	Display string `json:"display"`
	Talking bool   `json:"talking"`
	// TODO recheck is it "audio_codec" or "audiocodec"?
	AudioCodec string                `json:"audio_codec"`
	VideoCodec string                `json:"video_codec"`
	Streams    []PublisherStreamResp `json:"streams"`
}

type PublisherStreamResp struct {
	Type        string `json:"type"` // type of published stream, audio|video|data
	Mindex      int    `json:"mindex"`
	Mid         string `json:"mid"`
	Disabled    bool   `json:"disabled"` // if true, it means this stream is currently inactive (so codec, description, etc. will be missing) (rtp transceiver stopped)
	Codec       string `json:"codec"`
	Description string `json:"description"`
	Moderated   bool   `json:"moderated"`
	Simulcast   bool   `json:"simulcast"` // true if published stream uses simulcast (VP8 and H.264 only)
	SVC         bool   `json:"svc"`       // true if published stream uses SVC (VP9 only)
	Talking     bool   `json:"talking"`   // whether the publisher stream has audio activity or not (only if audio levels are used)
}

type EventUnpublishedMsg struct {
	VideoRoom   string `json:"videoroom"`
	Room        int64  `json:"room"`
	Unpublished int64  `json:"unpublished"`
}

type EventLeavingMsg struct {
	VideoRoom string `json:"videoroom"`
	Room      int64  `json:"room"`
	Leaving   int64  `json:"leaving"`
}

type EventPublishersMsg struct {
	VideoRoom  string          `json:"videoroom"`
	Room       int64           `json:"room"`
	Publishers []PublisherResp `json:"publishers"`
}

type PublisherJoinConfig map[string]interface{}

func (c PublisherJoinConfig) ID(id int64) PublisherJoinConfig {
	c["id"] = id //unique ID to register for the publisher; optional, will be chosen by the plugin if missing
	return c
}

func (c PublisherJoinConfig) Display(displayName string) PublisherJoinConfig {
	c["display"] = displayName // display name for the publisher
	return c
}

func (c PublisherJoinConfig) Token(token string) PublisherJoinConfig {
	c["token"] = token // invitation token, in case the room has an ACL
	return c
}

type PublisherPublishConfig map[string]interface{}

type PublisherMediaDescription struct {
	Mid         string `json:"mid"`
	Description string `json:"description"`
}

func (c PublisherPublishConfig) AudioCodec(audioCodec string) PublisherPublishConfig {
	c["audiocodec"] = audioCodec
	return c
}

func (c PublisherPublishConfig) VideoCodec(videoCodec string) PublisherPublishConfig {
	c["videocodec"] = videoCodec
	return c
}

func (c PublisherPublishConfig) Bitrate(bitrate int) PublisherPublishConfig {
	c["bitrate"] = bitrate
	return c
}

func (c PublisherPublishConfig) Record(record bool) PublisherPublishConfig {
	c["record"] = record
	return c
}

func (c PublisherPublishConfig) FileName(filename string) PublisherPublishConfig {
	c["filename"] = filename
	return c
}

func (c PublisherPublishConfig) Display(display string) PublisherPublishConfig {
	c["display"] = display
	return c
}

func (c PublisherPublishConfig) AudioLevelAverage(audioLevelAverage int) PublisherPublishConfig {
	c["audio_level_average"] = audioLevelAverage
	return c
}

func (c PublisherPublishConfig) AudioActivePackets(audioActivePackets int) PublisherPublishConfig {
	c["audio_active_packets"] = audioActivePackets
	return c
}

func (c PublisherPublishConfig) Descriptions(descriptions []PublisherMediaDescription) PublisherPublishConfig {
	c["descriptions"] = descriptions
	return c
}

type Publisher struct {
	ctx    context.Context
	id     int64
	handle *janus.Handle
	roomID int64

	OnPublishers  func(EventPublishersMsg)
	OnUnpublished func(EventUnpublishedMsg)
	OnLeaving     func(EventLeavingMsg)
}

func NewPublisher(ctx context.Context, handle *janus.Handle, roomID int64) *Publisher {
	p := &Publisher{
		ctx:    ctx,
		handle: handle,
		roomID: roomID,
	}
	go p.eventLoop()
	return p
}

func (p *Publisher) ID() int64 {
	return p.id
}

func (p *Publisher) Handle() *janus.Handle {
	return p.handle
}

// Join send join request, with the following:
// request data
// {
//   "request" : "join",
//   "ptype" : "publisher",
//   "room" : <unique ID of the room to join>,
//   "id" : <unique ID to register for the publisher; optional, will be chosen by the plugin if missing>,
//   "display" : "<display name for the publisher; optional>",
//   "token" : "<invitation token, in case the room has an ACL; optional>"
// }
func (p *Publisher) Join(config PublisherJoinConfig) error {
	req := map[string]interface{}{}
	for k, v := range config {
		req[k] = v
	}
	req["request"] = "join"
	req["ptype"] = TypePublisher
	req["room"] = p.roomID

	resp, err := p.handle.Message(req, nil)
	if err != nil {
		return err
	}

	pluginData := struct {
		VideoRoom   string          `json:"videoroom"`
		Room        int64           `json:"room"`
		Description string          `json:"description"`
		ID          int64           `json:"id"` // unique id of the participant
		PrivateID   int64           `json:"private_id"`
		Publishers  []PublisherResp `json:"publishers"`
		Attendees   []struct {
			ID      int64  `json:"id"`
			Display string `json:"display"`
		} `json:"attendees"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return err
	}

	if pluginData.VideoRoom != "joined" {
		return fmt.Errorf("unexpected response: videoroom %s", pluginData.VideoRoom)
	}

	p.id = pluginData.ID
	return nil
}

func (p *Publisher) Publish(sdpOffer string, config PublisherPublishConfig) (string, error) {
	req := map[string]interface{}{}
	for k, v := range config {
		req[k] = v
	}
	req["request"] = "publish"

	resp, err := p.handle.Message(req, &janus.JSEPData{
		Type: janus.SDPOffer,
		Sdp:  sdpOffer,
	})

	if err != nil {
		return "", err
	}

	if resp.JSEP == nil {
		log.Debug().Msg("videoroom:publish jsep nil")
		return "", errors.New("unexpected response: return nil jsep")
	}

	return resp.JSEP.Sdp, nil
}

func (p *Publisher) UnPublish() error {
	req := map[string]interface{}{
		"request": "unpublish",
	}

	_, err := p.handle.Message(req, nil)
	return err
}

func (p *Publisher) Configure(sdpOffer string) (string, error) {
	req := map[string]interface{}{
		"request": "configure",
	}

	resp, err := p.handle.Message(req, &janus.JSEPData{
		Type: janus.SDPOffer,
		Sdp:  sdpOffer,
	})
	if err != nil {
		return "", err
	}

	if resp.JSEP == nil {
		log.Debug().Msg("videoroom:configure jsep nil")
		return "", errors.New("unexpected response: return nil jsep")
	}
	return resp.JSEP.Sdp, nil
}

func (p *Publisher) Leave() error {
	req := map[string]interface{}{
		"request": "leave",
	}

	resp, err := p.handle.Message(req, nil)
	if err != nil {
		return err
	}

	pluginData := RawVideoRoomEventMsg{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return err
	}

	if pluginData.GetString("leaving") != "ok" {
		return errors.New("unexpected response: leaving != ok")
	}
	return nil
}

func (p *Publisher) eventLoop() {
	eventChan := p.handle.Events()
	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-eventChan:
			switch msg := evt.(type) {
			case *janus.EventMsg:
				var videoRoomMsg RawVideoRoomEventMsg
				_ = json.Unmarshal(msg.PluginData.Data, &videoRoomMsg)

				switch {
				case videoRoomMsg.Has("unpublished"):
					unpublishedMsg := EventUnpublishedMsg{}
					if err := json.Unmarshal(msg.PluginData.Data, &unpublishedMsg); err != nil {
						continue
					}
					if p.OnUnpublished != nil {
						p.OnUnpublished(unpublishedMsg)
					}
				case videoRoomMsg.Has("leaving"):
					leavingMsg := EventLeavingMsg{}
					if err := json.Unmarshal(msg.PluginData.Data, &leavingMsg); err != nil {
						continue
					}
					if p.OnLeaving != nil {
						p.OnLeaving(leavingMsg)
					}
				case videoRoomMsg.Has("publishers"):
					publishersMsg := EventPublishersMsg{}
					if err := json.Unmarshal(msg.PluginData.Data, &publishersMsg); err != nil {
						continue
					}
					if p.OnPublishers != nil {
						p.OnPublishers(publishersMsg)
					}
				}
			}
		}
	}
}
