package swarmdkg

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/swarm/api"
	"net/http"
)

type Feed struct {
	Topic string
	User  common.Address
	URL   string
}

func NewFeed(topic string, user common.Address, url string) *Feed {
	return &Feed{
		Topic: topic,
		User:  user,
		URL:   url,
	}
}

func (f *Feed) Read(manifestAddrHex string) ([]byte, error) {
	res, statusCode, err := GetRequestBZZ(f.URL, manifestAddrHex, "feed")
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("err %v", statusCode)
	}
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (f *Feed) GetManifest(manifestHash string) (*api.Manifest, error) {
	res, statusCode, err := GetRequestBZZ(f.URL, manifestHash, "raw")

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("err %v", statusCode)
	}

	manifest := &api.Manifest{}
	err = json.Unmarshal(res, manifest)
	if err != nil {
		return nil, err
	}

	if len(manifest.Entries) != 1 {
		return nil, fmt.Errorf("manifest has %d entries", len(manifest.Entries))
	}

	return manifest, nil
}
