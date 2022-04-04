package videoroom

import (
	"encoding/json"
	"fmt"
	"github.com/abdularis/janus-client-go/janus"
)

const PackageName = "janus.plugin.videoroom"

type UserType string

const (
	TypePublisher  UserType = "publisher"
	TypeSubscriber UserType = "subscriber"
)

type ParticipantInfoResp struct {
	ID        int64  `json:"id"`
	Display   string `json:"display"`
	Publisher bool   `json:"publisher"`
	Talking   bool   `json:"talking"`
}

type RawVideoRoomEventMsg map[string]json.RawMessage

func (m RawVideoRoomEventMsg) Has(key string) bool {
	_, ok := m[key]
	return ok
}

func (m RawVideoRoomEventMsg) GetString(key string) string {
	rawField, ok := m[key]
	if !ok {
		return ""
	}

	field := ""
	_ = json.Unmarshal(rawField, &field)
	return field
}

func (m RawVideoRoomEventMsg) IsVideoRoomEvent(event string) bool {
	return m.GetString("videoroom") == event
}

type CreateRoomConfig map[string]interface{}

func (c CreateRoomConfig) RoomID(roomID int64) CreateRoomConfig {
	c["room"] = roomID
	return c
}

func (c CreateRoomConfig) MaxPublishers(publishers int) CreateRoomConfig {
	c["publishers"] = publishers
	return c
}

func (c CreateRoomConfig) Permanent(permanent bool) CreateRoomConfig {
	c["permanent"] = permanent
	return c
}

func (c CreateRoomConfig) Description(description string) CreateRoomConfig {
	c["description"] = description
	return c
}

func (c CreateRoomConfig) Secret(secret string) CreateRoomConfig {
	c["secret"] = secret
	return c
}

func (c CreateRoomConfig) Pin(pin string) CreateRoomConfig {
	c["pin"] = pin
	return c
}

func (c CreateRoomConfig) IsPrivate(isPrivate bool) CreateRoomConfig {
	c["is_private"] = isPrivate
	return c
}

func (c CreateRoomConfig) Allowed(allowedTokens []string) CreateRoomConfig {
	c["allowed"] = allowedTokens
	return c
}

func CreateRoom(handle *janus.Handle, config CreateRoomConfig) error {
	req := map[string]interface{}{}
	for key, value := range config {
		req[key] = value
	}
	req["request"] = "create"

	resp, err := handle.Request(req)
	if err != nil {
		return err
	}

	msg := struct {
		VideoRoom string `json:"videoroom"`
		ErrorCode int    `json:"error_code"`
		Error     string `json:"error"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &msg); err != nil {
		return err
	}

	if msg.VideoRoom == "created" {
		return nil
	}

	return fmt.Errorf("(%d) %s", msg.ErrorCode, msg.Error)
}

func Exists(handle *janus.Handle, roomID int64) (bool, error) {
	resp, err := handle.Request(map[string]interface{}{
		"request": "exists",
		"room":    roomID,
	})
	if err != nil {
		return false, err
	}

	msg := struct {
		Room   int64 `json:"room"`
		Exists bool  `json:"exists"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &msg); err != nil {
		return false, err
	}

	return msg.Room == roomID && msg.Exists, nil
}

func ListParticipants(handle *janus.Handle, roomID int64) ([]ParticipantInfoResp, error) {
	req := map[string]interface{}{
		"request": "listparticipants",
		"room":    roomID,
	}
	resp, err := handle.Request(req)
	if err != nil {
		return nil, err
	}

	pluginData := struct {
		VideoRoom    string                `json:"videoroom"`
		Room         int64                 `json:"room"`
		Participants []ParticipantInfoResp `json:"participants"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return nil, err
	}

	if pluginData.VideoRoom != "participants" {
		return nil, fmt.Errorf("unexpected response videoroom event [%s]", pluginData.VideoRoom)
	}

	return pluginData.Participants, nil
}

func DestroyRoom(handle *janus.Handle, roomID int64, secret string, permanent bool) error {
	req := map[string]interface{}{
		"request":   "destroy",
		"room":      roomID,
		"secret":    secret,
		"permanent": permanent,
	}
	resp, err := handle.Request(req)
	if err != nil {
		return err
	}

	pluginData := RawVideoRoomEventMsg{}
	if err := json.Unmarshal(resp.PluginData.Data, &pluginData); err != nil {
		return err
	}

	if pluginData.GetString("videoroom") != "destroyed" {
		return fmt.Errorf("unexpected response while destroying room %d", roomID)
	}

	return nil
}
