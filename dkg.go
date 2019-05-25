package swarmdkg

import (
	"go.dedis.ch/kyber/pairing/bn256"
	"go.dedis.ch/kyber/util/key"
)

const (
	STATE_KEY_EXCHANGE = iota
	STATE_SEND_DEALS
	STATE_PROCESS_DEALS
)

type Streamer interface {
	Broadcast(msg []byte)
	Read() chan []byte
}

type DKG interface {
	ExchangePubkey() error
	// Phase I
	SendDeals() error
	ProcessDeals() error
	ProcessResponses() error
	ProcessJustifications() error

	// Phase II
	ProcessCommits() error
	ProcessComplaints() error
	ProcessReconstructCommits() error
}

func NewDkg(streamer Streamer, suite *bn256.Suite, numOfNodes, threshold int) *DKGInstance {
	return &DKGInstance{
		Streamer:   streamer,
		Suite:      suite,
		NumOfNodes: numOfNodes,
		Treshold:   threshold,
	}
}

type DKGInstance struct {
	Streamer   Streamer
	NumOfNodes int
	Treshold   int
	Suite      *bn256.Suite
}

func (i *DKGInstance) ExchangePubkey() error {
	keyPair := key.NewKeyPair(i.Suite)
	publicKeyBin, err := keyPair.Public.MarshalBinary()
	if err != nil {
		return err
	}
	i.Streamer.Broadcast(publicKeyBin)

	return nil
}
func (i *DKGInstance) SendDeals() error {
	return nil
}
func (i *DKGInstance) ProcessDeals() error {
	return nil
}
func (i *DKGInstance) ProcessResponses() error {
	return nil
}
func (i *DKGInstance) ProcessJustifications() error {
	return nil
}

// Phase II
func (i *DKGInstance) ProcessCommits() error {
	return nil
}
func (i *DKGInstance) ProcessComplaints() error {
	return nil
}
func (i *DKGInstance) ProcessReconstructCommits() error {
	return nil
}

func (i *DKGInstance) Run() error {
	return nil
}
