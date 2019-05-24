package swarmdkg

import (
	"testing"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"github.com/ethereum/go-ethereum/swarm/storage/feed/lookup"
	"net/url"
	"fmt"
	"io/ioutil"
	"bytes"
	"github.com/ethereum/go-ethereum/swarm/testutil"
	"github.com/ethereum/go-ethereum/swarm/storage"
	"encoding/json"
	"github.com/ethereum/go-ethereum/swarm/api"
	sr "github.com/ethereum/go-ethereum/swarm/api/http"
	"net/http"
	crypto "github.com/ethereum/go-ethereum/crypto"
)

// Test Swarm feeds using the raw update methods
func TestBzzFeed(t *testing.T) {
	srv := sr.NewTestSwarmServer(t, func(i *api.API) sr.TestServer {
		return sr.NewServer(i, "")
	}, nil)
	signer, _ := newTestSigner()

	defer srv.Close()

	// data of update 1
	update1Data := testutil.RandomBytes(1, 666)
	update1Timestamp := srv.CurrentTime
	//data for update 2
	update2Data := []byte("foo")

	topic, _ := feed.NewTopic("foo.eth", nil)
	updateRequest := feed.NewFirstRequest(topic)
	updateRequest.SetData(update1Data)

	if err := updateRequest.Sign(signer); err != nil {
		t.Fatal(err)
	}

	// creates feed and sets update 1
	testUrl, err := url.Parse(fmt.Sprintf("%s/bzz-feed:/", srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	urlQuery := testUrl.Query()
	body := updateRequest.AppendValues(urlQuery) // this adds all query parameters
	urlQuery.Set("manifest", "1")                // indicate we want a manifest back
	testUrl.RawQuery = urlQuery.Encode()

	resp, err := http.Post(testUrl.String(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	rsrcResp := &storage.Address{}
	err = json.Unmarshal(b, rsrcResp)
	if err != nil {
		t.Fatalf("data %s could not be unmarshaled: %v", b, err)
	}

	correctManifestAddrHex := "bb056a5264c295c2b0f613c8409b9c87ce9d71576ace02458160df4cc894210b"
	if rsrcResp.Hex() != correctManifestAddrHex {
		t.Fatalf("Response feed manifest mismatch, expected '%s', got '%s'", correctManifestAddrHex, rsrcResp.Hex())
	}

	// get the manifest
	testRawUrl := fmt.Sprintf("%s/bzz-raw:/%s", srv.URL, rsrcResp)
	resp, err = http.Get(testRawUrl)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	manifest := &api.Manifest{}
	err = json.Unmarshal(b, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("Manifest has %d entries", len(manifest.Entries))
	}
	correctFeedHex := "0x666f6f2e65746800000000000000000000000000000000000000000000000000c96aaa54e2d44c299564da76e1cd3184a2386b8d"
	if manifest.Entries[0].Feed.Hex() != correctFeedHex {
		t.Fatalf("Expected manifest Feed '%s', got '%s'", correctFeedHex, manifest.Entries[0].Feed.Hex())
	}

	// take the chance to have bzz: crash on resolving a feed update that does not contain
	// a swarm hash:
	testBzzUrl := fmt.Sprintf("%s/bzz:/%s", srv.URL, rsrcResp)
	resp, err = http.Get(testBzzUrl)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("Expected error status since feed update does not contain a Swarm hash. Received 200 OK")
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// get non-existent name, should fail
	testBzzResUrl := fmt.Sprintf("%s/bzz-feed:/bar", srv.URL)
	resp, err = http.Get(testBzzResUrl)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected get non-existent feed manifest to fail with StatusNotFound (404), got %d", resp.StatusCode)
	}

	resp.Body.Close()

	// get latest update through bzz-feed directly
	t.Log("get update latest = 1.1", "addr", correctManifestAddrHex)
	testBzzResUrl = fmt.Sprintf("%s/bzz-feed:/%s", srv.URL, correctManifestAddrHex)
	resp, err = http.Get(testBzzResUrl)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(update1Data, b) {
		t.Fatalf("Expected body '%x', got '%x'", update1Data, b)
	}

	// update 2
	// Move the clock ahead 1 second
	srv.CurrentTime++
	t.Log("update 2")

	// 1.- get metadata about this feed
	testBzzResUrl = fmt.Sprintf("%s/bzz-feed:/%s/", srv.URL, correctManifestAddrHex)
	resp, err = http.Get(testBzzResUrl + "?meta=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get feed metadata returned %s", resp.Status)
	}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	updateRequest = &feed.Request{}
	if err = updateRequest.UnmarshalJSON(b); err != nil {
		t.Fatalf("Error decoding feed metadata: %s", err)
	}
	updateRequest.SetData(update2Data)
	if err = updateRequest.Sign(signer); err != nil {
		t.Fatal(err)
	}
	testUrl, err = url.Parse(fmt.Sprintf("%s/bzz-feed:/", srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	urlQuery = testUrl.Query()
	body = updateRequest.AppendValues(urlQuery) // this adds all query parameters
	goodQueryParameters := urlQuery.Encode()    // save the query parameters for a second attempt

	// create bad query parameters in which the signature is missing
	urlQuery.Del("signature")
	testUrl.RawQuery = urlQuery.Encode()

	// 1st attempt with bad query parameters in which the signature is missing
	resp, err = http.Post(testUrl.String(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	expectedCode := http.StatusBadRequest
	if resp.StatusCode != expectedCode {
		t.Fatalf("Update returned %s. Expected %d", resp.Status, expectedCode)
	}

	// 2nd attempt with bad query parameters in which the signature is of incorrect length
	urlQuery.Set("signature", "0xabcd") // should be 130 hex chars
	resp, err = http.Post(testUrl.String(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	expectedCode = http.StatusBadRequest
	if resp.StatusCode != expectedCode {
		t.Fatalf("Update returned %s. Expected %d", resp.Status, expectedCode)
	}

	// 3rd attempt, with good query parameters:
	testUrl.RawQuery = goodQueryParameters
	resp, err = http.Post(testUrl.String(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	expectedCode = http.StatusOK
	if resp.StatusCode != expectedCode {
		t.Fatalf("Update returned %s. Expected %d", resp.Status, expectedCode)
	}

	// get latest update through bzz-feed directly
	t.Log("get update 1.2")
	testBzzResUrl = fmt.Sprintf("%s/bzz-feed:/%s", srv.URL, correctManifestAddrHex)
	resp, err = http.Get(testBzzResUrl)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(update2Data, b) {
		t.Fatalf("Expected body '%x', got '%x'", update2Data, b)
	}

	// test manifest-less queries
	t.Log("get first update in update1Timestamp via direct query")
	query := feed.NewQuery(&updateRequest.Feed, update1Timestamp, lookup.NoClue)

	urlq, err := url.Parse(fmt.Sprintf("%s/bzz-feed:/", srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	values := urlq.Query()
	query.AppendValues(values) // this adds feed query parameters
	urlq.RawQuery = values.Encode()
	resp, err = http.Get(urlq.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(update1Data, b) {
		t.Fatalf("Expected body '%x', got '%x'", update1Data, b)
	}

}


func newTestSigner() (*feed.GenericSigner, error) {
	privKey, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		return nil, err
	}
	return feed.NewGenericSigner(privKey), nil
}
