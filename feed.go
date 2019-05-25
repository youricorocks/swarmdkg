package swarmdkg

import "github.com/ethereum/go-ethereum/swarm/storage/feed"

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


// мы хтим слушать фиды списка пользователей
// мы хотим иметь фиды для разных событий

// Из стрима читаем и пишем в него. В нем должен жить и наш Feed
type Stream struct {
	Own MyFeed
	Feeds []Feed
}

type MyFeed struct{
	Feed
	feed.Signer
}

// Только читаем из фида
type Feed struct{}