package main

import (
	"context"
	"time"

	"github.com/maximiliano745/janus-client-go/janus"
	"github.com/maximiliano745/janus-client-go/janus/videoroom"

	//"github.com/maximiliano745/janus-client-go/sample/videoroom-publisher"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	janus.SetVerboseRequestResponse(true)
	var roomID int64 = 7555683579550993055

	gateway, err := janus.Connect("wss://janus-wa24.onrender.com")
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	_, _ = gateway.Info()

	session, err := gateway.Create()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	handle, err := session.Attach(videoroom.PackageName)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	go func() {
		t := time.NewTicker(time.Second * 50)
		for ; ; <-t.C {
			_ = session.KeepAlive()
		}
	}()

	exists, err := videoroom.Exists(handle, roomID)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	if !exists {
		config := videoroom.CreateRoomConfig{}.
			RoomID(roomID).
			MaxPublishers(100)
		err = videoroom.CreateRoom(handle, config)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
	}

	publisher := videoroom.NewPublisher(context.Background(), handle, roomID)

	wrtc := NewLocalWebRTCAgent("./sample/sample-video-scenery.ogg", "./sample/sample-video-scenery.ivf")
	wrtc.Start(publisher, roomID)
}
