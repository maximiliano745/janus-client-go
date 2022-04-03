package echotest

import (
	"github.com/abdularis/janus-client-go/janus"
)

const PackageName = "janus.plugin.echotest"

type EchoTest struct {
	handle *janus.Handle
}

func NewFromHandle(handle *janus.Handle) *EchoTest {
	return &EchoTest{handle: handle}
}

func (e *EchoTest) Trickle(candidates ...janus.IceCandidate) error {
	return e.handle.Trickle(candidates...)
}

func (e *EchoTest) Configure(sdpOffer string, enableAudio, enableVideo bool) (string, error) {
	resp, err := e.handle.Message(map[string]interface{}{
		"audio": enableAudio,
		"video": enableVideo,
	}, &janus.JSEPData{
		Type: "offer",
		Sdp:  sdpOffer,
	})
	if err != nil {
		return "", err
	}

	if resp.JSEP != nil {
		return resp.JSEP.Sdp, nil
	}

	return "", nil
}

func (e *EchoTest) EnableAudio(enable bool) error {
	_, err := e.handle.Message(map[string]interface{}{
		"audio": enable,
	}, nil)
	return err
}

func (e *EchoTest) EnableVideo(enable bool) error {
	_, err := e.handle.Message(map[string]interface{}{
		"video": enable,
	}, nil)
	return err
}
