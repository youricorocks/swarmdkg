package swarmdkg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/swarm/storage"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"io/ioutil"
	"net/http"
	"net/url"
)

type MyFeed struct {
	*Feed
	feed.Signer
	counter      int
	manifestHash string
}

func NewMyFeed(topic string, signer feed.Signer, url string) *MyFeed {
	return &MyFeed{
		Feed:   NewFeed(topic, signer.Address(), url),
		Signer: signer,
	}
}

func (own *MyFeed) Read() ([]byte, error) {
	if len(own.manifestHash) == 0 {
		return nil, errors.New("own.manifestHash should be initialised")
	}
	return own.Feed.Read(own.manifestHash)
}

func (own *MyFeed) Broadcast(msg []byte) error {
	if own.counter == 0 {
		manifestHash, err := own.firstUpdate(msg)
		if err != nil {
			return err
		}

		own.manifestHash = manifestHash.Hex()
		own.counter++
		fmt.Println("broadcast", own.User.String(), own.counter, own.manifestHash)
		return nil
	}

	res, statusCode, err := GetRequestBZZ(own.Feed.URL, own.manifestHash, "feed", "meta=1")
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("get feed metadata returned %v", statusCode)
	}

	updateRequest := &feed.Request{}
	if err := updateRequest.UnmarshalJSON(res); err != nil {
		return err
	}

	updateRequest.SetData(msg)
	if err := updateRequest.Sign(own.Signer); err != nil {
		return err
	}

	testUrl, err := url.Parse(fmt.Sprintf("%s/bzz-feed:/", own.URL))
	if err != nil {
		return err
	}

	urlQuery := testUrl.Query()
	body := updateRequest.AppendValues(urlQuery) // this adds all query parameters

	// update data with good query parameters:
	testUrl.RawQuery = urlQuery.Encode()

	resp, err := http.Post(testUrl.String(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("post feed returned %v", statusCode)
	}

	own.counter++
	return nil
}

func (own *MyFeed) ManifestHash() string {
	return own.manifestHash
}

func (own *MyFeed) firstUpdate(msg []byte) (*storage.Address, error) {
	topic, _ := feed.NewTopic(own.Topic, nil)
	updateRequest := feed.NewFirstRequest(topic)
	updateRequest.SetData(msg)

	if err := updateRequest.Sign(own.Signer); err != nil {
		return nil, err
	}

	// creates feed and sets update 1
	testUrl, err := url.Parse(fmt.Sprintf("%s/bzz-feed:/", own.URL))
	if err != nil {
		return nil, err
	}

	urlQuery := testUrl.Query()
	body := updateRequest.AppendValues(urlQuery) // this adds all query parameters
	urlQuery.Set("manifest", "1")                // indicate we want a manifest back
	testUrl.RawQuery = urlQuery.Encode()

	resp, err := http.Post(testUrl.String(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("err %s", resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rsrcResp := &storage.Address{}
	err = json.Unmarshal(b, rsrcResp)
	if err != nil {
		return nil, err
	}

	return rsrcResp, nil
}
