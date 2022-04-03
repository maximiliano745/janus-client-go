package audiobridge

import (
	"encoding/json"
	"fmt"
	"github.com/abdularis/janus-client-go/janus"
)

const PackageName = "janus.plugin.audiobridge"

func CreateRoom(handle *janus.Handle, roomID int64) error {
	resp, err := handle.Request(map[string]interface{}{
		"request": "create",
		"room":    roomID,
	})
	if err != nil {
		return err
	}

	// success resp
	//  {
	//  	"audiobridge": "created",
	//      "room": 100900,
	//      "permanent": false
	//  }
	//
	// error resp
	//  {
	//  	"audiobridge": "event",
	//      "error_code": 486,
	//      "error": "Room 100200 already exists"
	//  }
	msg := struct {
		AudioBridge string `json:"audiobridge"`
		ErrorCode   int    `json:"error_code"`
		Error       string `json:"error"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &msg); err != nil {
		return err
	}

	// success
	if msg.AudioBridge == "created" {
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

	// sample success response from plugin
	// {
	//    "audiobridge": "success",
	//    "room": 100200,
	//    "exists": true
	// }
	msg := struct {
		Room   int64 `json:"room"`
		Exists bool  `json:"exists"`
	}{}
	if err := json.Unmarshal(resp.PluginData.Data, &msg); err != nil {
		return false, err
	}

	return msg.Room == roomID && msg.Exists, nil
}

func Join(handle *janus.Handle, roomID int64, offerSDP string) (string, error) {
	resp, err := handle.Message(map[string]interface{}{
		"request": "join",
		"room":    roomID,
	}, &janus.JSEPData{
		Type: "offer",
		Sdp:  offerSDP,
	})
	if err != nil {
		return "", err
	}

	return resp.JSEP.Sdp, nil
}
