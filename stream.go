package swarmdkg

import "time"

type Stream struct {
	Own MyFeed
	Feeds []Feed
}

func NewStream(own MyFeed, feeds []Feed) Stream {
	return Stream{own, feeds}
}

func (s Stream) Broadcast(msg []byte) {
	s.Own.Broadcast(msg)
}

func (s Stream) Read() chan []byte {
	time.NewTicker(time.Second)
	s.Own.Read()

	for _, feed := range s.Feeds {
		feed.Read()
	}
}