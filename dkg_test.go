package swarmdkg

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/swarm/api"
	"github.com/ethereum/go-ethereum/swarm/api/http"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"github.com/ethereum/go-ethereum/swarm/testutil"
	"go.dedis.ch/kyber/pairing/bn256"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test Swarm feeds using the raw update methods
func TestBzzMyFeed(t *testing.T) {
	srv := http.NewTestSwarmServer(t, func(i *api.API) http.TestServer {
		return http.NewServer(i, "")
	}, nil)

	// data of update 1
	update1Data := testutil.RandomBytes(1, 666)
	update1Timestamp := srv.CurrentTime
	signer, _ := newTestSigner()

	myFeed := NewMyFeed("foo.eth", signer, srv.URL)

	defer srv.Close()

	// creates feed and sets update 1
	err := myFeed.Broadcast(update1Data)

	correctManifestAddrHex := "bb056a5264c295c2b0f613c8409b9c87ce9d71576ace02458160df4cc894210b"
	if myFeed.ManifestHash() != correctManifestAddrHex {
		t.Fatalf("Response feed manifest mismatch, expected '%s', got '%s'", correctManifestAddrHex, myFeed.ManifestHash())
	}

	// get the manifest
	manifest, err := myFeed.GetManifest(myFeed.ManifestHash())
	if err != nil {
		t.Fatal(err)
	}
	correctFeedHex := "0x666f6f2e65746800000000000000000000000000000000000000000000000000c96aaa54e2d44c299564da76e1cd3184a2386b8d"
	if manifest.Entries[0].Feed.Hex() != correctFeedHex {
		t.Fatalf("Expected manifest Feed '%s', got '%s'", correctFeedHex, manifest.Entries[0].Feed.Hex())
	}

	// get latest update through bzz-feed directly
	t.Log("get update latest = 1.1", "addr", correctManifestAddrHex)

	res, err := myFeed.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(update1Data, res) {
		t.Fatalf("Expected body '%x', got '%x'", update1Data, res)
	}

	// update 2
	// Move the clock ahead 1 second
	srv.CurrentTime++
	t.Log("update 2")

	update2Data := []byte("foo")
	err = myFeed.Broadcast(update2Data)
	if err != nil {
		t.Fatal(err)
	}

	// get latest update through bzz-feed directly
	t.Log("get update 1.2")
	// 1.- get metadata about this feed
	res, err = myFeed.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(update2Data, res) {
		t.Fatalf("Expected body '%x', got '%x'", update2Data, res)
	}

	// test manifest-less queries
	t.Log("get first update in update1Timestamp via direct query")
	foreignFeed := NewFeed("foo.eth", signer.Address(), srv.URL)
	res, err = foreignFeed.Get(update1Timestamp)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(update1Data, res) {
		t.Fatalf("Expected body '%x', got '%x'", update1Data, res)
	}
}

func TestBzzStream(t *testing.T) {
	srv := http.NewTestSwarmServer(t, func(i *api.API) http.TestServer {
		return http.NewServer(i, "")
	}, nil)

	const numUsers = 5
	var myFeeds []*MyFeed
	var updateData [][]byte

	// data of update 1
	update1Timestamp := srv.CurrentTime
	_ = update1Timestamp

	for i := 0; i < numUsers; i++ {
		signer, _ := newTestSigner()
		myFeeds = append(myFeeds, NewMyFeed("foo.eth", signer, srv.URL))
		updateData = append(updateData, testutil.RandomBytes(i, 20+i))
	}

	var streams []*Stream
	var closers []func()
	for i := range myFeeds {
		var streamFeeds []*Feed
		for j := 0; j < numUsers; j++ {
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

	defer func() {
		for _, closeFunc := range closers {
			closeFunc()
		}

		fmt.Println("*** Server is closed ***")
		srv.Close()
	}()

	// creates feed and sets update 1
	for i, stream := range streams {
		stream.Broadcast(updateData[i])
	}

	//wait for a broadcast
	time.Sleep(1 * time.Second)

	wg := sync.WaitGroup{}
	for i := range streams {
		wg.Add(1)

		go func(i int) {
			count := 0
			defer wg.Done()

			for {
				select {
				case msg := <-streams[i].Read():
					isWaited := false
					for _, data := range updateData {
						if bytes.Equal(msg, data) {
							isWaited = true
						}
					}
					if !isWaited {
						t.Fatal("stream got unexpected value", i, msg, updateData)
						return
					}

					count++
					if count == len(updateData) {
						// successful case
						fmt.Println("successful case")
						return
					}
				case <-time.After(5 * time.Second):
					t.Fatal("stream timeouted with", i, count)
					return
				}
			}
		}(i)

	}
	wg.Wait()
	fmt.Println("done")
}

var counter = new(int64)

func init() {
	*counter = 48879 //hex 'beef'
}

func newTestSigner() (*feed.GenericSigner, error) {
	tailBytes := fmt.Sprintf("%04x", atomic.AddInt64(counter, 1)-1)

	privKey, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdead" + tailBytes)
	if err != nil {
		return nil, err
	}
	return feed.NewGenericSigner(privKey), nil
}

func TestMockDKG(t *testing.T) {
	numOfDKGNodes := 4
	threshold := 3
	chans := NewReadChans(numOfDKGNodes)
	wg := sync.WaitGroup{}
	wg.Add(numOfDKGNodes)
	for i := 0; i < numOfDKGNodes; i++ {
		localI := i
		go func() {
			dkg := NewDkg(NewStreamerMock(chans, localI), bn256.NewSuiteG2(), numOfDKGNodes, threshold)
			err := dkg.Run()
			if err != nil {
				t.Log(err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}
