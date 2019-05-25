package swarmdkg

import (
	"strconv"
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

	// data of update 1
	update1Data := testutil.RandomBytes(1, 666)
	update1Timestamp := srv.CurrentTime

	topic, _ := feed.NewTopic("foo.eth", nil)
	updateRequest := feed.NewFirstRequest(topic)
	updateRequest.SetData(update1Data)

	signer, _ := newTestSigner()
	defer srv.Close()
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
	res, statusCode := testBZZGetRequest(t, srv.URL, rsrcResp.String(), "raw")
	if statusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}

	manifest := &api.Manifest{}
	err = json.Unmarshal(res, manifest)
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
	_, statusCode = testBZZGetRequest(t, srv.URL, rsrcResp.String())
	if statusCode == http.StatusOK {
		t.Fatal("Expected error status since feed update does not contain a Swarm hash. Received 200 OK")
	}

	// get latest update through bzz-feed directly
	t.Log("get update latest = 1.1", "addr", correctManifestAddrHex)

	res, statusCode = testBZZGetRequest(t, srv.URL, correctManifestAddrHex, "feed")
	if statusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
	if !bytes.Equal(update1Data, res) {
		t.Fatalf("Expected body '%x', got '%x'", update1Data, b)
	}

	// update 2
	// Move the clock ahead 1 second
	srv.CurrentTime++
	t.Log("update 2")

	// 1.- get metadata about this feed
	res, statusCode = testBZZGetRequest(t, srv.URL, correctManifestAddrHex, "feed", "meta=1")
	if statusCode != http.StatusOK {
		t.Fatalf("Get feed metadata returned %s", resp.Status)
	}

	updateRequest = &feed.Request{}
	if err = updateRequest.UnmarshalJSON(res); err != nil {
		t.Fatalf("Error decoding feed metadata: %s", err)
	}

	//data for update 2
	update2Data := []byte("foo")
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
	// 1.- get metadata about this feed
	res, statusCode = testBZZGetRequest(t, srv.URL, correctManifestAddrHex, "feed")
	if statusCode != http.StatusOK {
		t.Fatalf("Get feed returned %v", statusCode)
	}
	if !bytes.Equal(update2Data, res) {
		t.Fatalf("Expected body '%x', got '%x'", update2Data, res)
	}

	// test manifest-less queries
	t.Log("get first update in update1Timestamp via direct query")

	query := feed.NewQuery(&updateRequest.Feed, update1Timestamp, lookup.NoClue)
	res, statusCode = testBZZGetRequest(t, srv.URL, "", "feed",
		formQueryValue("time", strconv.FormatUint(query.TimeLimit, 10)),
		formQueryValue("topic", query.Topic.Hex()),
		formQueryValue("user", query.User.String()),
	)
	if statusCode != http.StatusOK {
		t.Fatalf("Get feed returned %v", statusCode)
	}

	if !bytes.Equal(update1Data, res) {
		t.Fatalf("Expected body '%x', got '%x'", update1Data, res)
	}

}

func newTestSigner() (*feed.GenericSigner, error) {
	privKey, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		return nil, err
	}
	return feed.NewGenericSigner(privKey), nil
}

// params[0] - resourceHash
// params[1] - additional base url (feed, raw, etc)
// params[2..] - additional query parameters in form "key=value"
func testBZZGetRequest(t *testing.T, url string, params...string) ([]byte, int)  {
	t.Helper()

	res, respCode, err := GetRequestBZZ(url, params...)
	if err != nil {
		t.Fatal(err)
	}

	return res, respCode
}

func formQueryValue(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}