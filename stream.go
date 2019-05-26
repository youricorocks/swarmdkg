package swarmdkg

import (
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"sync"
	"time"

	"github.com/JekaMas/awg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/swarm/api"
	"github.com/ethereum/go-ethereum/swarm/api/http"
)

type Stream struct {
	Own      *MyFeed
	Feeds    []*Feed
	Messages chan []byte
	cache    map[string]map[common.Address]map[string]struct{} //map[topic][user][msg]struct{}
	close    chan struct{}
	sync.Mutex
}

func NewStream(own *MyFeed, feeds []*Feed) *Stream {
	s := &Stream{
		Own:      own,
		Feeds:    feeds,
		Messages: make(chan []byte, 1024),
		cache:    make(map[string]map[common.Address]map[string]struct{}),
		close:    make(chan struct{}),
	}

	go func() {
		timer := time.NewTicker(1 * time.Second)
		defer timer.Stop()

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
						fmt.Println("Error while reading feed", err)
						return nil
					}

					if len(msg) == 0 {
						return nil
					}

					s.Lock()
					defer s.Unlock()

					_, ok := s.cache[feed.Topic]
					if !ok {
						s.cache[feed.Topic] = make(map[common.Address]map[string]struct{})
					}

					_, ok = s.cache[feed.Topic][feed.User]
					if !ok {
						s.cache[feed.Topic][feed.User] = make(map[string]struct{})
					}

					_, cached := s.cache[feed.Topic][feed.User][hex.EncodeToString(msg)]
					if cached {
						return nil
					}

					fmt.Println("GET FROM FEED", feed.User.String(), feed.Topic, msg)
					s.cache[feed.Topic][feed.User][hex.EncodeToString(msg)] = struct{}{}
					s.Messages <- msg

					return nil
				})
			}
			wg.Start()

			select {
			case <-s.close:
				return
			case t = <-timer.C:
				//nothing to do
			}
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

func (s *Stream) Close() {
	close(s.close)
}

func GenerateStreams(srv Server, signers []*feed.GenericSigner, topic string) (streams []*Stream, closer func()) {
	var myFeeds []*MyFeed
	for i := 0; i < len(signers); i++ {
		myFeeds = append(myFeeds, NewMyFeed(topic, signers[i], srv.URL()))
	}

	var closers []func()
	for i := range myFeeds {
		var streamFeeds []*Feed
		for j := 0; j < len(signers); j++ {
			if j == i {
				continue
			}
			f := myFeeds[j]
			streamFeeds = append(streamFeeds, NewFeed(f.Topic, f.User, f.URL))
		}

		stream := NewStream(myFeeds[i], streamFeeds)
		closers = append(closers, stream.Close)
		streams = append(streams, stream)
	}

	closer = func() {
		for _, closeFunc := range closers {
			closeFunc()
		}

		fmt.Println("*** Server is closed ***")
		//srv.Close()
	}

	return
}

func GenerateStream(srv Server, signers []*feed.GenericSigner, signerIdx int, topic string) (stream *Stream, closer func()) {
	myFeed := NewMyFeed(topic, signers[signerIdx], srv.URL())

	var closers []func()
	var streamFeeds []*Feed
	for i := 0; i < len(signers); i++ {
		if i == signerIdx {
			continue
		}
		streamFeeds = append(streamFeeds, NewFeed(topic, signers[i].Address(), srv.URL()))
	}

	stream = NewStream(myFeed, streamFeeds)
	closers = append(closers, stream.Close)

	closer = func() {
		for _, closeFunc := range closers {
			closeFunc()
		}

		fmt.Println("*** Server is closed ***")
		//srv.Close()
	}

	return
}

func GetTestServer() *TestServer {
	srv := http.NewTestSwarmServer(nil, func(i *api.API) http.TestServer {
		return http.NewServer(i, "")
	}, nil)

	return &TestServer{srv}
}

type TestServer struct {
	*http.TestSwarmServer
}

func (s *TestServer) URL() string {
	return s.TestSwarmServer.URL
}

func (s *TestServer) Close() {
	s.TestSwarmServer.Close()
}

type Server interface {
	URL() string
	Close()
}
