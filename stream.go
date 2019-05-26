package swarmdkg

import (
	"encoding/hex"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"sync"
	"time"

	"github.com/JekaMas/awg"
	"github.com/ethereum/go-ethereum/swarm/api"
	"github.com/ethereum/go-ethereum/swarm/api/http"
)

type Stream struct {
	Own      *MyFeed
	Feeds    []*Feed
	Messages chan []byte
	cache    map[string]struct{} //map[topic][user][msg]struct{}
	close    chan struct{}
	sync.Mutex
}

func NewStream(own *MyFeed, feeds []*Feed) *Stream {
	s := &Stream{
		Own:      own,
		Feeds:    feeds,
		Messages: make(chan []byte, 1024),
		cache:    make(map[string]struct{}),
		close:    make(chan struct{}),
	}

	timeCounter := uint64(time.Now().Unix()) - 10

	go func() {
		//fixme I'm not very sure could it skip a few updates or not
		timer := time.NewTicker(100 * time.Millisecond)
		defer timer.Stop()

		for {
			msg, err := s.Own.Read()
			if err != nil {
				//fmt.Println("Error while reading own feed", err)
			} else if len(msg) != 0 {
				s.Messages <- msg
			}

			wg := awg.AdvancedWaitGroup{}
			for _, feed := range s.Feeds {
				feed := feed

				wg.Add(func() error {
					msg, err = feed.Get(timeCounter)
					if err != nil {
						//fmt.Println("Error while reading feed", err)
						return nil
					}

					if len(msg) == 0 {
						return nil
					}

					s.Lock()
					defer s.Unlock()

					_, cached := s.cache[hex.EncodeToString(msg)]
					if cached {
						return nil
					}

					s.cache[hex.EncodeToString(msg)] = struct{}{}
					s.Messages <- msg

					return nil
				})
			}
			wg.Start()

			select {
			case <-s.close:
				return
			case <-timer.C:
				timeCounter++
			}
		}
	}()

	return s
}

func (s *Stream) Broadcast(msg []byte) {
	s.Own.Broadcast(msg)
	time.Sleep(2 * time.Second)
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

		//fmt.Println("*** Server is closed ***")
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

		//fmt.Println("*** Server is closed ***")
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
