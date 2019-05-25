package swarmdkg

import (
	"encoding/hex"
	"fmt"
	"github.com/JekaMas/awg"
	"github.com/ethereum/go-ethereum/common"
	"sync"
	"time"
)

type Stream struct {
	Own      *MyFeed
	Feeds    []*Feed
	Messages chan []byte
	cache map[string]map[common.Address]map[string]struct{} //map[topic][user][msg]struct{}
	sync.Mutex
}

func NewStream(own *MyFeed, feeds []*Feed) *Stream {
	s := &Stream{
		Own: own,
		Feeds: feeds,
		Messages: make(chan []byte, 1024),
		cache: make(map[string]map[common.Address]map[string]struct{}),
	}

	go func() {
		//fixme introduce context to cancel the goroutine
		timer := time.NewTicker(1 * time.Second)
		defer timer.Stop()

		// fixme do requests in goroutines
		t := time.Now()
		for {
			now := uint64(t.Unix())

			msg, err := s.Own.Read()
			if err != nil {
				fmt.Println("Error while reading own feed", err)
			} else if len(msg) != 0 {
				s.Messages <- msg
			}

			wg := awg.AdvancedWaitGroup{}
			for _, feed := range s.Feeds {
				feed := feed

				wg.Add(func() error {
					//fixme I'm not very sure could it skip a few updates or not
					msg, err = feed.Get(now)
					if err != nil {
						fmt.Println("Error while reading own feed", err)
						return nil
					}

					if len(msg) != 0 {
						s.Lock()
						defer s.Unlock()

						_, ok := s.cache[feed.Topic]
						if !ok {
							s.cache[feed.Topic] = make(map[common.Address]map[string]struct{})
						} else {
							_, ok = s.cache[feed.Topic][feed.User]
							if !ok {
								s.cache[feed.Topic][feed.User] = make(map[string]struct{})
							} else {
								_, cached := s.cache[feed.Topic][feed.User][hex.EncodeToString(msg)]
								if cached {
									return nil
								}
							}
						}

						s.cache[feed.Topic][feed.User][hex.EncodeToString(msg)] = struct{}{}
						s.Messages <- msg
					}
					return nil
				})
			}
			wg.Start()

			t = <-timer.C
		}
	}()

	return s
}

func (s *Stream) Broadcast(msg []byte) {
	s.Own.Broadcast(msg)
}

func (s *Stream) Read() chan []byte {
	return s.Messages
}
