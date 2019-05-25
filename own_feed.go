package swarmdkg

import "github.com/ethereum/go-ethereum/swarm/storage/feed"

type MyFeed struct{
	Feed
	feed.Signer
}

func NewMyFeed(topic string, signer feed.Signer) MyFeed {
	return MyFeed{NewFeed(topic, signer.Address()), signer}
}

func (own MyFeed) Broadcast(msg []byte) {

}
