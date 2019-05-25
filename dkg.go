package swarmdkg

import (
	"github.com/pkg/errors"
	"go.dedis.ch/kyber"
	"go.dedis.ch/kyber/pairing/bn256"
	"go.dedis.ch/kyber/util/key"
	"time"
)

const (
	TIMEOUT_FOR_STATE = 10 * time.Second

	STATE_PUBKEY_SEND = iota
	STATE_PUBKEY_RECEIVE
	STATE_SEND_DEALS
	STATE_PROCESS_DEALS
)

var timeoutErr = errors.New("timeout")

type Streamer interface {
	Broadcast(msg []byte)
	Read() chan []byte
}

type DKG interface {
	SendPubkey() error
	ReceivePubkeys() error
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
		State:      STATE_PUBKEY_SEND,

		pubkeys: make([]kyber.Point, 0, numOfNodes),
	}
}

type DKGInstance struct {
	Streamer   Streamer
	NumOfNodes int
	Treshold   int
	Suite      *bn256.Suite
	State      int
	KeyPair    *key.Pair

	pubkeys []kyber.Point
}

func (i *DKGInstance) SendPubkey() error {
	keyPair := key.NewKeyPair(i.Suite)
	publicKeyBin, err := keyPair.Public.MarshalBinary()
	if err != nil {
		return err
	}
	i.Streamer.Broadcast(publicKeyBin)
	i.State = STATE_PUBKEY_RECEIVE
	return nil
}

func (i *DKGInstance) ReceivePubkeys() error {
	ch := i.Streamer.Read()
	for {
		select {
		case key := <-ch:
			point := i.Suite.Point()
			err := point.UnmarshalBinary(key)
			if err != nil {
				i.pubkeys = i.pubkeys[:0]
				return err
			}
			i.pubkeys = append(i.pubkeys, point)
		case <-time.After(TIMEOUT_FOR_STATE):
			i.pubkeys = i.pubkeys[:0]
			return timeoutErr

		}
		if len(i.pubkeys) == i.NumOfNodes {
			break
		}
	}
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
	for {
		switch i.State {
		case STATE_PUBKEY_SEND:
			err := i.SendPubkey()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			i.moveToState(STATE_PUBKEY_RECEIVE)

		case STATE_PUBKEY_RECEIVE:
			err := i.ReceivePubkeys()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			i.moveToState(STATE_SEND_DEALS)
		case STATE_SEND_DEALS:
			err := i.SendDeals()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			i.moveToState(STATE_PROCESS_DEALS)

		default:
			return errors.New("unknown state")
		}
	}
	return nil
}
func (i *DKGInstance) moveToState(state int) {
	i.State = state
}
