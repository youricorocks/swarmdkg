package swarmdkg

import (
	"fmt"
	"time"
)

type Stream struct {
	Own      *MyFeed
	Feeds    []*Feed
	Messages chan []byte
}

func NewStream(own *MyFeed, feeds []*Feed) Stream {
	return Stream{own, feeds, make(chan []byte, 1024)}
}

func (s Stream) Broadcast(msg []byte) {
	s.Own.Broadcast(msg)
}

func (s Stream) Read() chan []byte {
	go func() {
		//fixme introduce context to cancel the goroutine
		timer := time.NewTicker(time.Second)
		defer timer.Stop()

		// fixme do requests in goroutines
		for t := range timer.C {
			msg, err := s.Own.Read()
			if err != nil {
				fmt.Println("Error while reading own feed", err)
			}
			if len(msg) != 0 {
				s.Messages <- msg
			}

			for _, feed := range s.Feeds {
				msg, err = feed.Get(uint64(t.Unix()))
				if err != nil {
					fmt.Println("Error while reading own feed", err)
				}
				if len(msg) != 0 {
					s.Messages <- msg
				}
			}
		}
	}()

	return s.Messages
}
