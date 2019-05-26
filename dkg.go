package swarmdkg

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/swarm/storage/feed"
	"go.dedis.ch/kyber"
	"go.dedis.ch/kyber/pairing/bn256"
	"go.dedis.ch/kyber/share"
	rabin "go.dedis.ch/kyber/share/dkg/rabin"
	"go.dedis.ch/kyber/share/vss/rabin"
	"go.dedis.ch/kyber/sign/bls"
	"go.dedis.ch/kyber/sign/tbls"
	"go.dedis.ch/kyber/util/key"
)

const (
	TIMEOUT_FOR_STATE = 20 * time.Second

	STATE_PUBKEY_SEND = iota
	STATE_PUBKEY_RECEIVE
	STATE_SEND_DEALS
	STATE_PROCESS_DEALS
	STATE_PROCESS_SEND_RESPONSES
	STATE_PROCESS_PROCESS_RESPONSES
)

var timeoutErr = errors.New("timeout")

type DKGMessage struct {
	From    int
	ToIndex int
	Data    []byte

	ReqID int
}

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

func NewDkg(srv Server, signerIdx int, suite *bn256.Suite, numOfNodes, threshold int) *DKGInstance {
	return &DKGInstance{
		Server:     srv,
		SignerIdx:  signerIdx,
		Suite:      suite,
		NumOfNodes: numOfNodes,
		Treshold:   threshold,
		State:      STATE_PUBKEY_SEND,

		pubkeys: make([]kyber.Point, 0, numOfNodes),
	}
}

type DKGInstance struct {
	Streamer   Streamer
	Server     Server
	SignerIdx  int
	NumOfNodes int
	Treshold   int
	Suite      *bn256.Suite
	State      int
	KeyPair    *key.Pair


	roundID int
	pubkeys  []kyber.Point
	Index    int
	DkgRabin *rabin.DistKeyGenerator
}

func (i *DKGInstance)round(k int)  {
	i.roundID=k
}
func (i *DKGInstance) SendPubkey() error {
	i.KeyPair = key.NewKeyPair(i.Suite)
	publicKeyBin, err := i.KeyPair.Public.MarshalBinary()
	if err != nil {
		return err
	}
	b, err := json.Marshal(DKGMessage{
		Data:publicKeyBin,
		ReqID:i.roundID,
	})
	if err != nil {
		return err
	}
	i.Streamer.Broadcast(b)
	time.Sleep(2*time.Second)
	return nil
}

func (i *DKGInstance) ReceivePubkeys() error {
	ch := i.Streamer.Read()
	for {
		select {
		case key := <-ch:
			msg:=DKGMessage{}
			json.Unmarshal(key, &msg)
			if msg.ReqID!=i.roundID {
				continue
			}
			point := i.Suite.Point()
			err := point.UnmarshalBinary(msg.Data)
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
	sort.Slice(i.pubkeys, func(k, m int) bool {
		return i.pubkeys[k].String() > i.pubkeys[m].String()
	})

	for j, p := range i.pubkeys {
		if p.Equal(i.KeyPair.Public) {
			i.Index = j
			break
		}
		if j == i.NumOfNodes-1 && i.Index == 0 {
			return errors.New("my key is not existed")
		}
	}

	var err error
	i.DkgRabin, err = rabin.NewDistKeyGenerator(i.Suite, i.KeyPair.Private, i.pubkeys, i.Treshold)
	if err != nil {
		return fmt.Errorf("Dkg instance init error: %v", err)
	}
	deals, err := i.DkgRabin.Deals()
	if err != nil {
		return fmt.Errorf("deal generation error: %v", err)
	}

	for toIndex, deal := range deals {
		fmt.Println("*** X", i.Index, toIndex, deal.Index, *deal.Deal)

		b := bytes.NewBuffer(nil)
		err = gob.NewEncoder(b).Encode(deal)
		if err != nil {
			return err
		}

		msg := DKGMessage{
			Data:    b.Bytes(),
			ToIndex: toIndex,
			From:    i.Index,
			ReqID:i.roundID,
		}
		msgBin, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		i.Streamer.Broadcast(msgBin)
		time.Sleep(2*time.Second)
	}
	return nil
}
func (i *DKGInstance) ProcessDeals() error {
	ch := i.Streamer.Read()
	numOfDeals := i.NumOfNodes - 1
	respList := make([]*rabin.Response, 0)
	m:=make(map[int]struct {})

	for {
		select {
		case deal := <-ch:
			var msg DKGMessage
			fmt.Println(i.Index, "** deal - ", string(deal))
			err := json.Unmarshal(deal, &msg)
			if err != nil {
				fmt.Println(i.Index, "0 ------- err", err)
				return err
			}
			if msg.ReqID!=i.roundID {
				fmt.Println("fuck round", deal)
				continue
			}
			if msg.ToIndex != i.Index {
				continue
			}
			_,ok:=m[msg.From]
			if ok {
				continue
			}
			m[msg.From]= struct{}{}

			dd := &rabin.Deal{
				Deal: &vss.EncryptedDeal{
					DHKey: i.Suite.Point(),
				},
			}

			dec := gob.NewDecoder(bytes.NewBuffer(msg.Data))
			err = dec.Decode(dd)
			if err != nil {
				fmt.Println(i.Index, "1 ------- err", err)
				return err
			}

			resp, err := i.DkgRabin.ProcessDeal(dd)
			if err != nil {
				fmt.Println("*** 1", deal)
				fmt.Println("*** 2", msg.From, msg.ToIndex, dd.Index, *dd.Deal)
				fmt.Println("fuck 3")
				return err
			}
			fmt.Println("*** 3", msg.From, msg.ToIndex, dd.Index, *dd.Deal)
			fmt.Println("*** 4", deal)
			respList = append(respList, resp)
			numOfDeals--
			fmt.Println("+++", i.Index, numOfDeals)

		case <-time.After(TIMEOUT_FOR_STATE):
			i.pubkeys = i.pubkeys[:0]
			return timeoutErr

		}
		if numOfDeals == 0 {
			break
		}
	}

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

var signers []*feed.GenericSigner
var signersLock = new(sync.Mutex)

func (i *DKGInstance) Run() error {
	signersLock.Lock()
	if signers == nil {
		var err error
		signers, err = newTestSigners(i.NumOfNodes)
		fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", err)
	}
	signersLock.Unlock()

	var closerFunc func()

	for {
		switch i.State {
		case STATE_PUBKEY_SEND:
			i.Streamer, closerFunc = GenerateStream(i.Server, signers, i.SignerIdx, "pubkey")
			fmt.Println("xxx 1", i.SignerIdx, i.Index, i.Streamer)
			time.Sleep(2 * time.Second)

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

			closerFunc()
			i.moveToState(STATE_SEND_DEALS)
		case STATE_SEND_DEALS:
			i.Streamer, closerFunc = GenerateStream(i.Server, signers, i.SignerIdx, "deals")
			fmt.Println("xxx 2", i.SignerIdx, i.Index, i.Streamer)
			time.Sleep(2 * time.Second)

			err := i.SendDeals()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			i.moveToState(STATE_PROCESS_DEALS)
		case STATE_PROCESS_DEALS:
			err := i.ProcessDeals()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}

			closerFunc()
			i.moveToState(STATE_PROCESS_SEND_RESPONSES)

		default:
			fmt.Println("default Exit")
			return errors.New("unknown state")
		}
	}
	return nil
}
func (i *DKGInstance) moveToState(state int) {
	fmt.Println("Move form", i.State, "to", state)
	i.State = state
	time.Sleep(2 * time.Second)
}

func (i *DKGInstance) GetVerifier() (*BLSVerifier, error) {
	if i.DkgRabin == nil || !i.DkgRabin.Finished() {
		return nil, errors.New("not ready yet")
	}

	distKeyShare, err := i.DkgRabin.DistKeyShare()
	if err != nil {
		return nil, fmt.Errorf("failed to get DistKeyShare: %v", err)
	}

	var (
		masterPubKey = share.NewPubPoly(bn256.NewSuiteG2(), nil, distKeyShare.Commitments())
		newShare     = &BLSShare{
			ID:   i.Index,
			Pub:  &share.PubShare{I: i.Index, V: i.KeyPair.Public},
			Priv: distKeyShare.PriShare(),
		}
	)

	return NewBLSVerifier(masterPubKey, newShare, i.Treshold, i.NumOfNodes), nil
}

type BLSShare struct {
	ID   int
	Pub  *share.PubShare
	Priv *share.PriShare
}

type BLSVerifier struct {
	Keypair      *BLSShare // This verifier's BLSShare.
	masterPubKey *share.PubPoly
	suiteG1      *bn256.Suite
	suiteG2      *bn256.Suite
	t            int
	n            int
}

func NewBLSVerifier(masterPubKey *share.PubPoly, sh *BLSShare, t, n int) *BLSVerifier {
	return &BLSVerifier{
		masterPubKey: masterPubKey,
		Keypair:      sh,
		suiteG1:      bn256.NewSuiteG1(),
		suiteG2:      bn256.NewSuiteG2(),
		t:            t,
		n:            n,
	}
}

func (m *BLSVerifier) Sign(data []byte) ([]byte, error) {
	sig, err := tbls.Sign(m.suiteG1, m.Keypair.Priv, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sing random data with key %v %v with error %v", m.Keypair.Pub, data, err)
	}

	return sig, nil
}

func (m *BLSVerifier) VerifyRandomShare(prevRandomData, currRandomData []byte) error {
	// Check that the signature itself is correct for this validator.
	if err := tbls.Verify(m.suiteG1, m.masterPubKey, prevRandomData, currRandomData); err != nil {
		return fmt.Errorf("signature of share is corrupt: %v. prev random: %v; current random: %v", err, prevRandomData, currRandomData)
	}

	return nil
}

func (m *BLSVerifier) VerifyRandomData(prevRandomData, currRandomData []byte) error {
	if err := bls.Verify(m.suiteG1, m.masterPubKey.Commit(), prevRandomData, currRandomData); err != nil {
		return fmt.Errorf("signature is corrupt: %v. prev random: %v; current random: %v", err, prevRandomData, currRandomData)
	}

	return nil
}

func (m *BLSVerifier) Recover(msg []byte, sigs [][]byte) ([]byte, error) {
	aggrSig, err := tbls.Recover(m.suiteG1, m.masterPubKey, msg, sigs, m.t, m.n)
	if err != nil {
		return nil, fmt.Errorf("failed to recover aggregate signature: %v", err)
	}

	return aggrSig, nil
}
