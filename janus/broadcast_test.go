package janus

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"sync/atomic"
	"testing"
	"time"
)

func listen(id string, cb *chanBroadcast, counter *int32) {
	c := cb.subscribe()
	defer func() {
		cb.unsubscribe(c)
	}()

	for {
		msg := <-c
		atomic.AddInt32(counter, 1)
		fmt.Printf("%s <- %s\n", id, msg)
		break
	}
}

func TestChanBroadcast_PubSub(t *testing.T) {
	var counter int32 = 0
	cb := newChanBroadcast()
	go listen("1", cb, &counter)
	go listen("2", cb, &counter)
	go listen("3", cb, &counter)

	assert.Equal(t, int32(0), counter)

	time.Sleep(time.Second)
	assert.Equal(t, 3, len(cb.channels))

	cb.publish("Hello")

	time.Sleep(time.Second)
	assert.Equal(t, 0, len(cb.channels))
	assert.Equal(t, int32(3), counter)
}
