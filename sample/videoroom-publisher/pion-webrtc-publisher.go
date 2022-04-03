package main

import (
	"context"
	"fmt"
	"github.com/abdularis/janus-client-go/janus"
	"github.com/abdularis/janus-client-go/janus/videoroom"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/ivfreader"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"time"
)

const (
	oggPageDuration = time.Millisecond * 20
)

type LocalWebRTCAgent struct {
	audioFileName string
	videoFileName string
}

func NewLocalWebRTCAgent(
	audioFileName string,
	videoFileName string) *LocalWebRTCAgent {
	return &LocalWebRTCAgent{
		audioFileName: audioFileName,
		videoFileName: videoFileName,
	}
}

func (w *LocalWebRTCAgent) Start(publisher *videoroom.Publisher, roomID int64) {
	_, err := os.Stat(w.videoFileName)
	haveVideoFile := !os.IsNotExist(err)

	_, err = os.Stat(w.audioFileName)
	haveAudioFile := !os.IsNotExist(err)

	if !haveAudioFile && !haveVideoFile {
		panic("Could not find `" + w.audioFileName + "` or `" + w.videoFileName + "`")
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	if haveVideoFile {
		// Create a video track
		videoTrack, videoTrackErr := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
		if videoTrackErr != nil {
			panic(videoTrackErr)
		}

		rtpSender, videoTrackErr := peerConnection.AddTrack(videoTrack)
		if videoTrackErr != nil {
			panic(videoTrackErr)
		}

		// Read incoming RTCP packets
		// Before these packets are returned they are processed by interceptors. For things
		// like NACK this needs to be called.
		go func() {
			rtcpBuf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					return
				}
			}
		}()

		go func() {
			// Open a IVF file and start reading using our IVFReader
			file, ivfErr := os.Open(w.videoFileName)
			if ivfErr != nil {
				panic(ivfErr)
			}

			ivf, header, ivfErr := ivfreader.NewWith(file)
			if ivfErr != nil {
				panic(ivfErr)
			}

			// Wait for connection established
			<-iceConnectedCtx.Done()

			// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
			// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
			//
			// It is important to use a time.Ticker instead of time.Sleep because
			// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
			// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
			ticker := time.NewTicker(time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000))
			for ; true; <-ticker.C {
				frame, _, ivfErr := ivf.ParseNextFrame()
				if ivfErr == io.EOF {
					fmt.Printf("All video frames parsed and sent")
					os.Exit(0)
				}

				if ivfErr != nil {
					panic(ivfErr)
				}

				if ivfErr = videoTrack.WriteSample(media.Sample{Data: frame, Duration: time.Second}); ivfErr != nil {
					panic(ivfErr)
				}
			}
		}()
	}

	if haveAudioFile {
		log.Println("add audio file track")
		// Create a audio track
		audioTrack, audioTrackErr := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
		if audioTrackErr != nil {
			panic(audioTrackErr)
		}

		rtpSender, audioTrackErr := peerConnection.AddTrack(audioTrack)
		if audioTrackErr != nil {
			panic(audioTrackErr)
		}

		// Read incoming RTCP packets
		// Before these packets are returned they are processed by interceptors. For things
		// like NACK this needs to be called.
		go func() {
			rtcpBuf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					return
				}
			}
		}()

		go func() {
			// Open a OGG file and start reading using our OGGReader
			file, oggErr := os.Open(w.audioFileName)
			if oggErr != nil {
				panic(oggErr)
			}

			// Open on oggfile in non-checksum mode.
			ogg, _, oggErr := oggreader.NewWith(file)
			if oggErr != nil {
				panic(oggErr)
			}

			// Wait for connection established
			<-iceConnectedCtx.Done()

			// Keep track of last granule, the difference is the amount of samples in the buffer
			var lastGranule uint64

			// It is important to use a time.Ticker instead of time.Sleep because
			// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
			// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
			ticker := time.NewTicker(oggPageDuration)
			for ; true; <-ticker.C {
				pageData, pageHeader, oggErr := ogg.ParseNextPage()
				if oggErr == io.EOF {
					fmt.Printf("All audio pages parsed and sent")
					os.Exit(0)
				}

				if oggErr != nil {
					panic(oggErr)
				}

				// The amount of samples is the difference between the last and current timestamp
				sampleCount := float64(pageHeader.GranulePosition - lastGranule)
				lastGranule = pageHeader.GranulePosition
				sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

				if oggErr = audioTrack.WriteSample(media.Sample{Data: pageData, Duration: sampleDuration}); oggErr != nil {
					panic(oggErr)
				}
			}
		}()
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
		}
	})

	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

		if s == webrtc.PeerConnectionStateFailed {
			// Wait until PeerConnection has had no network activity for 30 seconds or another failure. It may be reconnected using an ICE Restart.
			// Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
			// Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}
	})

	var readyToSendIce = false
	var tempIceCandidates []janus.IceCandidate
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			ice := candidate.ToJSON()
			iceCand := janus.IceCandidate{
				SdpMid:        *ice.SDPMid,
				SdpMLineIndex: int(*ice.SDPMLineIndex),
				Candidate:     ice.Candidate,
			}
			if readyToSendIce {
				err := publisher.Handle().Trickle(iceCand)
				if err != nil {
					log.Printf("trickel ice err: %s\n", err)
				}
			} else {
				tempIceCandidates = append(tempIceCandidates, iceCand)
			}
			fmt.Printf("ice candidate %+v\n", candidate.ToJSON())

		}
	})

	// -- nego nego
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Fatal(err)
	}

	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		log.Fatal(err)
	}

	err = publisher.Join(nil)
	if err != nil {
		log.Fatal(err)
	}

	localSdp := peerConnection.LocalDescription()
	answerSdp, err := publisher.Publish(localSdp.SDP, nil)
	if err != nil {
		log.Fatal(err)
	}

	readyToSendIce = true
	if len(tempIceCandidates) > 0 {
		log.Printf("sending %d candidates\n", len(tempIceCandidates))
		err := publisher.Handle().Trickle(tempIceCandidates...)
		if err != nil {
			log.Printf("err trickle temp candidates: %s\n", err)
		}
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSdp,
	}
	err = peerConnection.SetRemoteDescription(answer)
	if err != nil {
		log.Fatal(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	fmt.Println("block forever")

	// Block forever
	select {}
}
