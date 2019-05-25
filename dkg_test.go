package swarmdkg

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/swarm/api"
	sr "github.com/ethereum/go-ethereum/swarm/api/http"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"github.com/ethereum/go-ethereum/swarm/testutil"
	"go.dedis.ch/kyber/pairing/bn256"
	"sync"
	"sync/atomic"
	"testing"
)

// Test Swarm feeds using the raw update methods
func TestBzzMyFeed(t *testing.T) {
	srv := sr.NewTestSwarmServer(t, func(i *api.API) sr.TestServer {
		return sr.NewServer(i, "")
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
	srv := sr.NewTestSwarmServer(t, func(i *api.API) sr.TestServer {
		return sr.NewServer(i, "")
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

var counter = new(int64)
func init() {
	*counter = 48879 //hex 'beef'
}

func newTestSigner() (*feed.GenericSigner, error) {
	tailBytes := fmt.Sprintf("%04x", atomic.AddInt64(counter, 1)-1)

	privKey, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdead"+tailBytes)
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
