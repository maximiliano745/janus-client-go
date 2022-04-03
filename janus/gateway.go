package janus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/segmentio/ksuid"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var ErrTimeout = errors.New("request timeout")
var ErrUnexpected = errors.New("unexpected error")

const defaultRequestTimeout = 10

type ConnectionState int

const (
	Disconnected ConnectionState = iota + 1
	Connected
	Reconnecting
)

var isDebug = false

func SetDebug(debug bool) {
	isDebug = debug
}

// Gateway represent connection to janus server
// ref: https://janus.conf.meetecho.com/docs/rest.html#root
type Gateway struct {
	janusUrl            string
	connMtx             sync.Mutex
	conn                *websocket.Conn
	connState           ConnectionState
	sessionsMtx         sync.RWMutex
	sessions            map[int64]*Session
	trxMtx              sync.RWMutex
	reqTransactions     map[string]chan interface{}
	closeWsFunc         context.CancelFunc
	reconnectCancelFunc context.CancelFunc
	logger              *log.Entry
}

func generateRequestTransactionID() string {
	return ksuid.New().String()
}

// Connect create and connect to janus server
func Connect(janusUrl string) (*Gateway, error) {
	u, err := url.Parse(janusUrl)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("url scheme must be ws:// or wss://")
	}

	g := &Gateway{
		janusUrl:        janusUrl,
		sessions:        make(map[int64]*Session),
		reqTransactions: make(map[string]chan interface{}),
		logger:          log.WithField("host", u.Host),
	}
	if err = g.connectWs(); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *Gateway) connectWs() error {
	header := http.Header{
		"Sec-WebSocket-Protocol": {"janus-protocol"},
	}
	c, _, err := websocket.DefaultDialer.Dial(g.janusUrl, header)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	g.closeWsFunc = cancel
	g.conn = c
	g.connState = Connected
	go g.ping(ctx)
	go g.receive()
	return nil
}

func (g *Gateway) ReconnectWs(ctx context.Context) {
	if g.connState == Reconnecting {
		return
	}
	g.connState = Reconnecting

	delayTime := time.Duration(2)
	for {
		g.logger.Infof("reconnect ws in %ds", delayTime)
		t := time.NewTimer(time.Second * delayTime)
		select {
		case <-ctx.Done():
			g.logger.Info("reconnect cancelled")
			return
		case <-t.C:
			err := g.connectWs()
			if err != nil {
				g.logger.WithError(err).Info("reconnect error")
				delayTime = (delayTime * 2) + time.Duration(rand.Intn(5))
				continue
			}
			g.logger.Info("reconnect success")
			g.reconnectCancelFunc = nil
			g.reclaimSessions()
			return
		}
	}
}

func (g *Gateway) reclaimSessions() {
	g.sessionsMtx.Lock()
	defer g.sessionsMtx.Unlock()

	invalidCount := 0
	for key, session := range g.sessions {
		err := session.claim()
		if err != nil && !session.IsValid() {
			delete(g.sessions, key)
		}
	}

	g.logger.Infof("reclaim sessions: invalid(%d) reclaimed(%d)", invalidCount, len(g.sessions))
}

func (g *Gateway) invalidateSessions() {
	g.sessionsMtx.Lock()
	defer g.sessionsMtx.Unlock()
	for _, session := range g.sessions {
		session.setIsValid(false)
	}
}

func (g *Gateway) URL() string {
	return g.janusUrl
}

func (g *Gateway) IsConnected() bool {
	return g.connState == Connected
}

// Create a new session
func (g *Gateway) Create() (*Session, error) {
	body := map[string]interface{}{
		"janus": "create",
	}

	resp, err := g.sendRequestSync(body)
	if err != nil {
		return nil, err
	}

	switch resp := resp.(type) {
	case *SuccessMsg:
		session := &Session{
			id:      resp.Data.ID,
			events:  newChanBroadcast(),
			handles: make(map[int64]*Handle),
			gateway: g,
			logger:  g.logger.WithField("session", resp.Data.ID),
			isValid: true,
			destroyHook: func(sessionID int64) {
				g.deleteSession(sessionID)
			},
		}
		g.addSession(session)
		return session, nil
	case *ErrorMsg:
		return nil, resp
	default:
		return nil, ErrUnexpected
	}
}

// Info return janus server info
func (g *Gateway) Info() (*ServerInfoMsg, error) {
	body := map[string]interface{}{
		"janus": "info",
	}

	resp, err := g.sendRequestSync(body)
	if err != nil {
		return nil, err
	}

	switch resp := resp.(type) {
	case *ServerInfoMsg:
		return resp, nil
	case *ErrorMsg:
		return nil, resp
	default:
		return nil, ErrUnexpected
	}
}

// Close connection to janus gateway server
func (g *Gateway) Close() error {
	if g.reconnectCancelFunc != nil {
		g.reconnectCancelFunc()
	}

	for _, sess := range g.sessions {
		_ = sess.Destroy()
	}
	g.connState = Disconnected
	return g.conn.Close()
}

func (g *Gateway) addSession(session *Session) {
	g.sessionsMtx.Lock()
	defer g.sessionsMtx.Unlock()
	g.sessions[session.id] = session
}

func (g *Gateway) deleteSession(sessionID int64) {
	g.sessionsMtx.Lock()
	defer g.sessionsMtx.Unlock()
	delete(g.sessions, sessionID)
}

// sendSync send request to janus server and wait for response message
// if waitForEventMsg is true we expect the flow will be "send message --wait-> receive ack msg --wait-> receive event msg"
// if waitForEventMsg is false we expect the flow "send request --wait-> receive success/error msg"
func (g *Gateway) sendSync(body map[string]interface{}, waitForEventMsg bool) (interface{}, error) {
	reqTrxID := generateRequestTransactionID()
	body["transaction"] = reqTrxID

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	if isDebug {
		debugData, _ := json.MarshalIndent(body, "", "    ")
		fmt.Printf("[DEBUG][%s] 🍎 --> send, raw: %s\n", g.janusUrl, debugData)
	}

	responseCh := make(chan interface{})
	g.addTransaction(reqTrxID, responseCh)
	defer g.removeTransaction(reqTrxID)

	g.connMtx.Lock()
	err = g.conn.WriteMessage(websocket.TextMessage, data)
	g.connMtx.Unlock()
	if err != nil {
		return nil, err
	}

	t := time.NewTimer(time.Second * defaultRequestTimeout)
	select {
	case msg := <-responseCh:
		if !waitForEventMsg {
			return msg, nil
		}

		switch msg := msg.(type) {
		case *AckMsg:
			return g.waitForEventAfterAck(responseCh)
		default:
			return msg, nil
		}
	case <-t.C:
		return nil, ErrTimeout
	}
}

// sendRequestSync convenient func to send regular synchronous request to janus server
func (g *Gateway) sendRequestSync(body map[string]interface{}) (interface{}, error) {
	return g.sendSync(body, false)
}

// sendMessageSync convenient func to
// send message to plugin handle which will be handle by janus plugin synchronously or asynchronously
// send message -> receive ack -> receive event
func (g *Gateway) sendMessageSync(body map[string]interface{}) (interface{}, error) {
	return g.sendSync(body, true)
}

func (g *Gateway) waitForEventAfterAck(responseCh chan interface{}) (interface{}, error) {
	t := time.NewTimer(time.Second * defaultRequestTimeout)
	select {
	case msg := <-responseCh:
		return msg, nil
	case <-t.C:
		// notes: should we return previous ack msg instead of timeout error?
		return nil, ErrTimeout
	}
}

func (g *Gateway) ping(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			g.logger.Info("ping close")
			return
		case <-ticker.C:
			g.logger.Info("ping janus")
			g.connMtx.Lock()
			err := g.conn.WriteControl(websocket.PingMessage, []byte("-ping-"), time.Now().Add(20*time.Second))
			g.connMtx.Unlock()
			if err != nil {
				g.logger.WithError(err).Error("ping error")
				return
			}
		}
	}
}

func (g *Gateway) getTransaction(transactionID string) chan interface{} {
	g.trxMtx.RLock()
	defer g.trxMtx.RUnlock()
	return g.reqTransactions[transactionID]
}

func (g *Gateway) addTransaction(transactionID string, responseCh chan interface{}) {
	g.trxMtx.Lock()
	defer g.trxMtx.Unlock()
	g.reqTransactions[transactionID] = responseCh
}

func (g *Gateway) removeTransaction(transactionID string) {
	g.trxMtx.Lock()
	defer g.trxMtx.Unlock()
	delete(g.reqTransactions, transactionID)
}

func (g *Gateway) receive() {
	defer func() {
		g.connState = Disconnected
		if g.closeWsFunc != nil {
			g.closeWsFunc()
		}
	}()

	for {
		_, data, err := g.conn.ReadMessage()

		if err != nil {
			// if its disconnect or other error that makes this loop break
			// mark all sessions and its handles as invalid
			g.invalidateSessions()

			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				g.logger.WithError(err).Info("close normally")
				return
			}

			// reconnect if its not intentionally disconnected
			if g.connState != Disconnected {
				reconnectCtx, reconnectCancelFunc := context.WithCancel(context.Background())
				g.reconnectCancelFunc = reconnectCancelFunc
				go g.ReconnectWs(reconnectCtx)
			}
			return
		}

		var baseMsg GeneralMsg
		if err := json.Unmarshal(data, &baseMsg); err != nil {
			continue
		}

		if isDebug {
			fmt.Printf("[DEBUG][%s] 🍏 <-- recv, raw: %s\n", g.janusUrl, data)
		}

		typeFunc, ok := msgTypesMapper[baseMsg.Janus]
		if !ok {
			g.logger.
				WithField("msg", baseMsg).
				Errorf("invalid janus message type received: %s", baseMsg.Janus)
			continue
		}

		msg := typeFunc()
		if err := json.Unmarshal(data, msg); err != nil {
			g.logger.WithError(err).Debugf("error unmarshall to specific message type")
			continue
		}

		trx := g.getTransaction(baseMsg.Transaction)
		// message will be deliver as part of request transaction or to async event channel for session/plugin handle
		if trx == nil {
			// note:
			// edge case, when detach plugin handle the current handle instance may already removed from map
			// before receiving 'detached' event, therefore the event will be ignored
			session := g.sessions[baseMsg.SessionID]
			if session == nil {
				continue
			}

			// send event to session
			if baseMsg.Sender == 0 {
				go session.passMsg(msg)
				continue
			}

			// send event to plugin handle
			handle := session.Handle(baseMsg.Sender)
			if handle != nil {
				go handle.passMsg(msg)
			}
		} else {
			// there's issue when janus 'event' message received before 'ack' message, therefore trx already resolved
			// but the channel might still exist on trx map
			go passMsgWithTimeout(trx, msg)
		}
	}
}

func passMsgWithTimeout(c chan interface{}, msg interface{}) {
	t := time.NewTimer(time.Second * 2)
	select {
	case c <- msg:
	case <-t.C:
		break
	}
}
