package swarmdkg

import (
	"github.com/ethereum/go-ethereum/common"
)

type Feed struct {
	Topic string
	User  common.Address
}

func NewFeed(topic string, user common.Address) Feed {
	return Feed{topic, user}
}

func (f Feed) Read() []byte {
	return []byte{}
}
