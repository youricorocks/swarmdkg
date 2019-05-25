package swarmdkg

import "time"

type Stream struct {
	Own   MyFeed
	Feeds []Feed
	Messages  chan []byte
}

func NewStream(own MyFeed, feeds []Feed) Stream {
	return Stream{own, feeds, make(chan []byte, 1024)}
}

func (s Stream) Broadcast(msg []byte) {
	s.Own.Broadcast(msg)
}

func (s Stream) Read() chan []byte {
	go func() {
		timer := time.NewTicker(time.Second)
		defer timer.Stop()

		for range timer.C {
			msg := s.Own.Read()
			if len(msg) != 0 {
				s.Messages <- msg
			}

			for _, feed := range s.Feeds {
				if msg = feed.Read(); len(msg) != 0 {
					s.Messages <- msg
				}
			}
		}
	}()

	return s.Messages
}