package swarmdkg

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
)

/*
// Phase I
	SendDeals,
	ProcessDeals,
	ProcessResponses,
	ProcessJustifications,

// Phase II
	ProcessCommits,
	ProcessComplaints,
	ProcessReconstructCommits,
*/

type Feed struct{
	Topic string
	User common.Address
}

func NewFeed(topic string, user common.Address) Feed {
	return Feed{topic, user}
}

func (f Feed) Read() []byte {

}