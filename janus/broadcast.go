package janus

import (
	"github.com/rs/zerolog/log"
	"sync"
	"time"
)

type chanAny chan interface{}
type chanAnyReadOnly <-chan interface{}

type chanBroadcast struct {
	channels map[chanAnyReadOnly]chanAny
	m        sync.RWMutex
}

func newChanBroadcast() *chanBroadcast {
	return &chanBroadcast{
		channels: make(map[chanAnyReadOnly]chanAny),
	}
}

func (c *chanBroadcast) publish(msg interface{}) {
	c.m.RLock()
	defer c.m.RUnlock()
	for _, ch := range c.channels {
		go c.passMsg(ch, msg)
	}
}

func (c *chanBroadcast) passMsg(ch chan interface{}, msg interface{}) {
	t := time.NewTimer(time.Second * 2)
	select {
	case ch <- msg:
		return
	case <-t.C:
		log.Info().Msg("channel broadcast pass msg timeout")
		return
	}
}

func (c *chanBroadcast) subscribe() <-chan interface{} {
	c.m.Lock()
	defer c.m.Unlock()
	newChan := make(chan interface{})
	c.channels[newChan] = newChan
	return newChan
}

func (c *chanBroadcast) unsubscribe(ch <-chan interface{}) {
	c.m.Lock()
	defer c.m.Unlock()
	if _, ok := c.channels[ch]; ok {
		delete(c.channels, ch)
	}
}
