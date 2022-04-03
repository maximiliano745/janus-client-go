package main

import (
	"context"
	"github.com/abdularis/janus-client-go/janus"
	"github.com/abdularis/janus-client-go/janus/videoroom"
	log "github.com/sirupsen/logrus"
	"time"
)

func main() {
	janus.SetDebug(true)
	log.SetLevel(log.DebugLevel)
	var roomID int64 = 7555683579550993055

	gateway, err := janus.Connect("ws://localhost:8188")
	if err != nil {
		log.Fatal(err)
	}

	_, _ = gateway.Info()

	session, err := gateway.Create()
	if err != nil {
		log.Fatal(err)
	}

	handle, err := session.Attach(videoroom.PackageName)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		t := time.NewTicker(time.Second * 50)
		for ; ; <-t.C {
			_ = session.KeepAlive()
		}
	}()

	exists, err := videoroom.Exists(handle, roomID)
	if err != nil {
		log.Fatal(err)
	}

	if !exists {
		config := videoroom.CreateRoomConfig{}.
			RoomID(roomID).
			MaxPublishers(100)
		err = videoroom.CreateRoom(handle, config)
		if err != nil {
			log.Fatal(err)
		}
	}

	publisher := videoroom.NewPublisher(context.Background(), handle, roomID)

	wrtc := NewLocalWebRTCAgent("./sample/sample-video-scenery.ogg", "./sample/sample-video-scenery.ivf")
	wrtc.Start(publisher, roomID)
}
