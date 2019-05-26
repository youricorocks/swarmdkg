package swarmdkg

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/swarm/api"
	"github.com/ethereum/go-ethereum/swarm/api/http"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"github.com/ethereum/go-ethereum/swarm/testutil"
	"go.dedis.ch/kyber/pairing/bn256"
	"strconv"
	"sync"
	"testing"
	"time"
	"math/rand"
)

// Test Swarm feeds using the raw update methods
func TestBzzMyFeed(t *testing.T) {
	t.SkipNow()
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
	t.SkipNow()
	const numUsers = 5
	var updateData [][]byte
	for i := 0; i < numUsers; i++ {
		updateData = append(updateData, testutil.RandomBytes(i, 20+i))
	}

	streams, closerFunc := getStreams(t, numUsers, "some-topic")
	defer closerFunc()

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

func TestBzzStreamBroadcastGetManyTimes(t *testing.T) {
	t.SkipNow()
	const numUsers = 5
	streams, closerFunc := getStreams(t, numUsers, "some-topic")
	defer closerFunc()

	for idx := 0; idx < 3; idx++ {
		var updateData [][]byte
		for i := 0; i < numUsers; i++ {
			updateData = append(updateData, testutil.RandomBytes(i+idx, 20+i+idx))
		}

		wg := sync.WaitGroup{}
		wg.Add(len(streams))
		for i := range streams {
			i := i
			go func() {
				streams[i].Broadcast(updateData[i])
				wg.Done()
			}()
		}
		wg.Wait()

		//wait for a broadcast
		time.Sleep(2 * time.Second)

		wg = sync.WaitGroup{}
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
							fmt.Println("stream got unexpected value", i, msg, updateData)
							continue
						}

						count++
						if count == len(updateData) {
							// successful case
							fmt.Println("successful case")
							return
						}
					case <-time.After(5 * time.Second):
						fmt.Println("stream timeouted with", i, count)
						t.Fatal("stream timeouted with", i, count)
						return
					}
				}
			}(i)
		}
		wg.Wait()
	}

	fmt.Println("done")
}

func TestBzzStreamBroadcastGetManyTimesManyStreams(t *testing.T) {
	t.SkipNow()
	const numUsers = 5

	for streamCount := 0; streamCount < 2; streamCount++ {
		streams, closerFunc := getStreams(t, numUsers, "some-topic"+strconv.Itoa(streamCount))

		for idx := 0; idx < 3; idx++ {
			var updateData [][]byte
			for i := 0; i < numUsers; i++ {
				updateData = append(updateData, testutil.RandomBytes(i+idx+streamCount, 20+i+idx))
			}

			wg := sync.WaitGroup{}
			wg.Add(len(streams))
			for i := range streams {
				i := i
				go func() {
					streams[i].Broadcast(updateData[i])
					wg.Done()
				}()
			}
			wg.Wait()

			//wait for a broadcast
			time.Sleep(2 * time.Second)

			wg = sync.WaitGroup{}
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
								fmt.Println("stream got unexpected value", i, msg, updateData)
								continue
							}

							count++
							if count == len(updateData) {
								// successful case
								fmt.Println("successful case")
								return
							}
						case <-time.After(5 * time.Second):
							fmt.Println("stream timeouted with", i, count)
							t.Fatal("stream timeouted with", i, count)
							return
						}
					}
				}(i)
			}
			wg.Wait()
		}
		closerFunc()
		fmt.Println("done stream")
	}

	fmt.Println("done")
}

func TestDKG(t *testing.T) {
	numOfDKGNodes := 4
	threshold := 3

	var signers []*feed.GenericSigner
	for idx := 0; idx < numOfDKGNodes; idx++ {
		s, _ := newTestSigner()
		signers = append(signers, s)
	}
	roundID:=rand.Int()
	wg := sync.WaitGroup{}
	wg.Add(numOfDKGNodes)
	srv := GetTestServer()
	var dkgs []*DKGInstance
	for i := 0; i < numOfDKGNodes; i++ {
		localI := i
		go func() {
			dkg := NewDkg(srv, localI, bn256.NewSuiteG2(), numOfDKGNodes, threshold)
			dkg.round(roundID)
			dkgs = append(dkgs, dkg)
			err := dkg.Run()
			if err != nil {
				t.Log(err)
			}
			wg.Done()

			go func() {
				// generate random stage
				dkg := dkgs[localI]

				verifier, err := dkg.GetVerifier()
				if err != nil {
					//t.Log(err)
				}

				randomRound := 0
				previousRandom := []byte("some initial vector")
				for {
					stream, closerFunc := GenerateStream(dkg.Server, signers, dkg.SignerIdx, "random"+strconv.Itoa(randomRound))
					time.Sleep(2*time.Second)

					mySign, err := verifier.Sign(previousRandom)
					if err != nil {
						//fmt.Println("+++ random 1", err)
						continue
					}

					stream.Broadcast(mySign)

					signsCache := make(map[string]struct{})

					got := 0
					var signs [][]byte
					for msg := range stream.Read() {
						if _, ok := signsCache[hex.EncodeToString(msg)]; ok {
							continue
						}
						signsCache[hex.EncodeToString(msg)] = struct{}{}

						err = verifier.VerifyRandomShare(previousRandom, msg)
						if err != nil {
							fmt.Println("+++ random 2", err)
							continue
						} else {
							signs = append(signs, msg)
							got++
						}

						if got == numOfDKGNodes {
							break
						}
					}

					newRandom, err := verifier.Recover(previousRandom, signs)
					if err != nil {
						fmt.Println("+++ random 3", err)
						continue
					}

					fmt.Printf("DONE Random round %d - random %s\n", randomRound, hex.EncodeToString(newRandom))

					closerFunc()
					randomRound++
					previousRandom = newRandom
				}
			}()
		}()
	}
	wg.Wait()

	time.Sleep(10*time.Minute)
}

func getStreams(t *testing.T, numUsers int, topic string) (streams []*Stream, closer func()) {
	t.Helper()

	srv := http.NewTestSwarmServer(t, func(i *api.API) http.TestServer {
		return http.NewServer(i, "")
	}, nil)

	var myFeeds []*MyFeed
	for i := 0; i < numUsers; i++ {
		signer, _ := newTestSigner()
		myFeeds = append(myFeeds, NewMyFeed(topic, signer, srv.URL))
	}

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

	closer = func() {
		for _, closeFunc := range closers {
			closeFunc()
		}

		fmt.Println("*** Stream is closed ***")
		srv.Close()
	}

	return
}
