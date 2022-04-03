package janus

import (
	log "github.com/sirupsen/logrus"
	"sync"
)

// Session represent communication session inside gateway connection
// ref: https://janus.conf.meetecho.com/docs/rest.html#sessions
type Session struct {
	id          int64
	events      *chanBroadcast
	handlesMtx  sync.RWMutex
	handles     map[int64]*Handle
	destroyHook func(sessionID int64)
	gateway     *Gateway
	isValid     bool
	logger      *log.Entry
}

func (s *Session) passMsg(msg interface{}) {
	switch msg.(type) {
	case *TimeoutMsg:
		// note: maybe check in gateway or call hook to remove this invalid session
		s.logger.Info("timeout, invalidate session")
		s.setIsValid(false)
	}
	s.events.publish(msg)
}

func (s *Session) setIsValid(isValid bool) {
	s.isValid = isValid
	s.handlesMtx.Lock()
	defer s.handlesMtx.Unlock()
	for _, handle := range s.handles {
		handle.isValid = isValid
	}
}

func (s *Session) ID() int64 {
	return s.id
}

func (s *Session) Events() <-chan interface{} {
	return s.events.subscribe()
}

func (s *Session) Handle(handleID int64) *Handle {
	s.handlesMtx.RLock()
	defer s.handlesMtx.RUnlock()
	return s.handles[handleID]
}

func (s *Session) IsValid() bool {
	return s.isValid
}

func (s *Session) KeepAlive() error {
	body := map[string]interface{}{
		"janus":      "keepalive",
		"session_id": s.id,
	}

	resp, err := s.gateway.sendRequestSync(body)
	if err != nil {
		return err
	}

	switch resp := resp.(type) {
	case *AckMsg:
		// keep alive ack (success)
		return nil
	case *ErrorMsg:
		return resp
	default:
		return ErrUnexpected
	}
}

// Attach current session to particular plugin
func (s *Session) Attach(pluginPackage string) (*Handle, error) {
	body := map[string]interface{}{
		"janus":      "attach",
		"session_id": s.id,
		"plugin":     pluginPackage,
	}

	resp, err := s.gateway.sendRequestSync(body)
	if err != nil {
		return nil, err
	}

	switch resp := resp.(type) {
	case *SuccessMsg:
		handle := &Handle{
			id:        resp.Data.ID,
			sessionID: s.id,
			events:    newChanBroadcast(),
			gateway:   s.gateway,
			isValid:   true,
			logger:    s.logger.WithField("handle", resp.Data.ID),
			detachHook: func(handleID int64) {
				s.removeHandle(handleID)
			},
		}
		s.addHandle(handle)
		return handle, nil
	case *ErrorMsg:
		return nil, resp
	default:
		return nil, ErrUnexpected
	}
}

// Destroy existing session
func (s *Session) Destroy() error {
	s.detachAllHandles()

	body := map[string]interface{}{
		"janus":      "destroy",
		"session_id": s.id,
	}

	resp, err := s.gateway.sendRequestSync(body)
	if err != nil {
		return err
	}

	switch resp := resp.(type) {
	case *SuccessMsg:
		s.setIsValid(false)
		s.destroyHook(s.id)
		return nil
	case *ErrorMsg:
		return resp
	default:
		return ErrUnexpected
	}
}

// claim try to reclaim current session in janus server in case transport disconnected
// to enable reclaim edit reclaim_session_timeout to be non-zero value in janus.jcfg file
// otherwise session and its handlers will immediately destroyed after transport is gone
func (s *Session) claim() error {
	body := map[string]interface{}{
		"janus":      "claim",
		"session_id": s.id,
	}
	resp, err := s.gateway.sendRequestSync(body)
	if err != nil {
		return err
	}

	switch resp := resp.(type) {
	case *ErrorMsg:
		if resp.Err.Code == 458 {
			s.setIsValid(false)
			s.logger.WithError(resp).Debug("invalidate session")
		}
		return resp
	default:
		s.setIsValid(true)
		return nil
	}
}

func (s *Session) detachAllHandles() {
	s.handlesMtx.RLock()
	defer s.handlesMtx.RUnlock()
	for _, handle := range s.handles {
		_ = handle.Detach()
	}
}

func (s *Session) addHandle(handle *Handle) {
	s.handlesMtx.Lock()
	defer s.handlesMtx.Unlock()
	s.handles[handle.id] = handle
}

func (s *Session) removeHandle(handleID int64) {
	s.handlesMtx.Lock()
	defer s.handlesMtx.Unlock()
	delete(s.handles, handleID)
}
