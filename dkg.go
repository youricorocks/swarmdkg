package swarmdkg

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
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
	STATE_SEND_RESPONSES
	STATE_PROCESS_RESPONSES
	STATE_PROCESS_JUSTIFICATIONS
	STATE_PROCESS_Commits
	STATE_PROCESS_Complaints
	STATE_PROCESS_ReconstructCommits
)
const (
	MESSAGE_DEAL = iota
	MESSAGE_RESPONSE
	MESSAGE_JUSTIFICATION
	MESSAGE_SECRET_COMMITS
	MESSAGE_COMPLAINS
)

var timeoutErr = errors.New("timeout")

type DKGMessage struct {
	From    int
	ToIndex int
	Type    int
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
	SendResponses() error
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

	roundID   int
	pubkeys   []kyber.Point
	responses []*rabin.Response
	complains []*rabin.ComplaintCommits
	Index     int
	DkgRabin  *rabin.DistKeyGenerator
}

func (i *DKGInstance) round(k int) {
	i.roundID = k
}
func (i *DKGInstance) SendPubkey() error {
	i.KeyPair = key.NewKeyPair(i.Suite)
	publicKeyBin, err := i.KeyPair.Public.MarshalBinary()
	if err != nil {
		return err
	}
	b, err := json.Marshal(DKGMessage{
		Data:  publicKeyBin,
		ReqID: i.roundID,
	})
	if err != nil {
		return err
	}
	i.Streamer.Broadcast(b)
	time.Sleep(2 * time.Second)
	return nil
}

func (i *DKGInstance) ReceivePubkeys() error {
	ch := i.Streamer.Read()
	for {
		select {
		case k := <-ch:
			msg := DKGMessage{}
			json.Unmarshal(k, &msg)
			if msg.ReqID != i.roundID {
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
		if len(i.pubkeys) == i.NumOfNodes*20 {
			i.pubkeys = uniquePublicKeys(i.pubkeys)

			sort.Slice(i.pubkeys, func(k, m int) bool {
				d1, _ := i.pubkeys[k].MarshalBinary()
				d2, _ := i.pubkeys[m].MarshalBinary()
				res := bytes.Compare(d1, d2)

				return res > 0
			})

			break
		}
	}
	return nil
}

func uniquePublicKeys(pubkeys []kyber.Point) []kyber.Point {
	keys := make(map[string]struct{})
	var list []kyber.Point
	for _, pubkey := range pubkeys {
		data, _ := pubkey.MarshalBinary()
		dataHex := hex.EncodeToString(data)

		if _, value := keys[dataHex]; !value {
			keys[dataHex] = struct{}{}
			list = append(list, pubkey)
		}
	}
	return list
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
			Type:    MESSAGE_DEAL,
			ReqID:   i.roundID,
		}
		msgBin, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		i.Streamer.Broadcast(msgBin)
	}
	return nil
}
func (i *DKGInstance) ProcessDeals() error {
	ch := i.Streamer.Read()
	numOfDeals := i.NumOfNodes - 1
	respList := make([]*rabin.Response, 0)
	dealsCache := make(map[string]struct{})

	for {
		select {
		case deal := <-ch:
			if _, ok := dealsCache[hex.EncodeToString(deal)]; ok {
				fmt.Println("old deal")
				continue
			}
			dealsCache[hex.EncodeToString(deal)] = struct{}{}

			var msg DKGMessage
			fmt.Println(i.Index, "** deal - ", string(deal))
			err := json.Unmarshal(deal, &msg)
			if err != nil {
				fmt.Println(i.Index, "0 ------- err", err)
				return err
			}
			if msg.ReqID != i.roundID {
				fmt.Println("fuck round", deal)
				continue
			}
			if msg.ToIndex != i.Index {
				continue
			}

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
			i.responses = append(i.responses, resp)
			fmt.Println("*** 3", msg.From, msg.ToIndex, dd.Index, *dd.Deal)
			fmt.Println("*** 4", numOfDeals, deal)
			respList = append(respList, resp)
			numOfDeals--
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
func (i *DKGInstance) SendResponses() error {
	for j := range i.responses {
		buf := bytes.NewBuffer(nil)
		err := gob.NewEncoder(buf).Encode(i.responses[j])
		if err != nil {
			return err
		}

		msg := DKGMessage{
			From: i.Index,
			Type: MESSAGE_RESPONSE,
			Data: buf.Bytes(),
		}

		b, err := json.Marshal(&msg)
		if err != nil {
			return err
		}
		i.Streamer.Broadcast(b)
	}
	i.responses = i.responses[:0]
	return nil
}
func (i *DKGInstance) ProcessResponses() error {
	ch := i.Streamer.Read()
	numOfResponses := (i.NumOfNodes - 1) * (i.NumOfNodes - 1)
	just := make([]*rabin.Justification, 0)

	for {
		select {
		case resp := <-ch:
			var msg DKGMessage
			err := json.Unmarshal(resp, &msg)
			if err != nil {
				return err
			}

			if msg.Type != MESSAGE_RESPONSE {
				continue
			}
			r := &rabin.Response{}

			dec := gob.NewDecoder(bytes.NewBuffer(msg.Data))
			err = dec.Decode(r)
			if err != nil {
				return err
			}

			if uint32(i.Index) == r.Response.Index {
				continue
			}
			j, err := i.DkgRabin.ProcessResponse(r)
			if err != nil {
				return err
			}

			just = append(just, j)
			numOfResponses--

		case <-time.After(TIMEOUT_FOR_STATE):
			i.pubkeys = i.pubkeys[:0]
			return timeoutErr

		}
		if numOfResponses == 0 {
			break
		}
	}
	for j := range just {
		var data []byte
		if just[j] != nil {
			buf := bytes.NewBuffer(nil)
			err := gob.NewEncoder(buf).Encode(just[j])
			if err != nil {
				return err
			}
			data = buf.Bytes()
		}

		msg := DKGMessage{
			From: i.Index,
			Type: MESSAGE_JUSTIFICATION,
			Data: data,
		}

		b, err := json.Marshal(&msg)
		if err != nil {
			return err
		}
		i.Streamer.Broadcast(b)
	}
	return nil
}
func (i *DKGInstance) ProcessJustifications() error {
	ch := i.Streamer.Read()
	numOfJustifications := (i.NumOfNodes - 1) * (i.NumOfNodes - 1) * i.NumOfNodes

	for {
		if numOfJustifications == 0 {
			break
		}

		select {
		case resp := <-ch:
			var msg DKGMessage

			err := json.Unmarshal(resp, &msg)
			if err != nil {
				return err
			}

			if msg.Type != MESSAGE_JUSTIFICATION {
				continue
			}

			if len(msg.Data) == 0 {
				numOfJustifications--
				continue
			}

			r := &rabin.Justification{}
			dec := gob.NewDecoder(bytes.NewBuffer(msg.Data))
			err = dec.Decode(r)
			if err != nil {
				return err
			}
			err = i.DkgRabin.ProcessJustification(r)
			if err != nil {
				return err
			}
			numOfJustifications--

		case <-time.After(TIMEOUT_FOR_STATE):
			i.pubkeys = i.pubkeys[:0]
			return timeoutErr
		}
	}

	return nil
}

// Phase II
func (i *DKGInstance) ProcessCommits() error {
	commits, err := i.DkgRabin.SecretCommits()
	if err != nil {
		return err
	}
	fmt.Println("sc len", len(commits.Commitments))

	buf := bytes.NewBuffer(nil)
	err = gob.NewEncoder(buf).Encode(commits)
	if err != nil {
		return err
	}

	msg := DKGMessage{
		From: i.Index,
		Type: MESSAGE_SECRET_COMMITS,
		Data: buf.Bytes(),
	}

	b, err := json.Marshal(&msg)
	if err != nil {
		return err
	}
	i.Streamer.Broadcast(b)

	commitCache := make(map[string]struct{})

	ch := i.Streamer.Read()
	numOfCommits := i.NumOfNodes - 1
	for {
		select {
		case commit := <-ch:
			if _, ok := commitCache[hex.EncodeToString(commit)]; ok {
				continue
			}
			commitCache[hex.EncodeToString(commit)] = struct{}{}

			var msg DKGMessage
			err := json.Unmarshal(commit, &msg)
			if err != nil {
				return err
			}
			if msg.From == i.Index {
				continue
			}

			if msg.Type != MESSAGE_SECRET_COMMITS {
				continue
			}
			fmt.Println(i.Index, msg.From, string(commit))
			commitData := &rabin.SecretCommits{}
			for j := 0; j < len(i.DkgRabin.QUAL())-1; j++ {
				commitData.Commitments = append(commitData.Commitments, i.Suite.Point())
			}
			dec := gob.NewDecoder(bytes.NewBuffer(msg.Data))
			err = dec.Decode(commitData)
			if err != nil {
				return err
			}

			complain, err := i.DkgRabin.ProcessSecretCommits(commitData)
			if err != nil {
				return err
			}
			i.complains = append(i.complains, complain)
			numOfCommits--
		case <-time.After(TIMEOUT_FOR_STATE):
			i.pubkeys = i.pubkeys[:0]
			return timeoutErr

		}
		if numOfCommits == 0 {
			break
		}
	}

	for j := range i.complains {
		var data []byte
		if i.complains[j] != nil {
			buf := bytes.NewBuffer(nil)
			err = gob.NewEncoder(buf).Encode(i.complains[j])
			if err != nil {
				return err
			}
			data = buf.Bytes()
		}

		msg := DKGMessage{
			From: i.Index,
			Type: MESSAGE_COMPLAINS,
			Data: data,
		}

		b, err := json.Marshal(&msg)
		if err != nil {
			return err
		}
		i.Streamer.Broadcast(b)
	}
	i.complains = i.complains[:0]

	return nil
}
func (i *DKGInstance) ProcessComplaints() error {
	//skipped for MVP
	return nil
}
func (i *DKGInstance) ProcessReconstructCommits() error {
	//skipped for MVP
	return nil
}

var signers []*feed.GenericSigner
var signersLock = new(sync.Mutex)

func (i *DKGInstance) Run() error {
	signersLock.Lock()
	if signers == nil {
		signers, _ = newTestSigners(i.NumOfNodes)
	}
	signersLock.Unlock()

	for {
		switch i.State {
		case STATE_PUBKEY_SEND:
			i.Streamer, _ = GenerateStream(i.Server, signers, i.SignerIdx, "pubkey")
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

			i.moveToState(STATE_SEND_DEALS)
		case STATE_SEND_DEALS:
			i.Streamer, _ = GenerateStream(i.Server, signers, i.SignerIdx, "deals")
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

			i.moveToState(STATE_SEND_RESPONSES)
		case STATE_SEND_RESPONSES:
			i.Streamer, _ = GenerateStream(i.Server, signers, i.SignerIdx, "responses")
			time.Sleep(2 * time.Second)

			err := i.SendResponses()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			i.moveToState(STATE_PROCESS_RESPONSES)

		case STATE_PROCESS_RESPONSES:
			err := i.ProcessResponses()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}

			i.moveToState(STATE_PROCESS_JUSTIFICATIONS)
		case STATE_PROCESS_JUSTIFICATIONS:
			err := i.ProcessJustifications()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}

			i.moveToState(STATE_PROCESS_Commits)
		case STATE_PROCESS_Commits:
			i.Streamer, _ = GenerateStream(i.Server, signers, i.SignerIdx, "commits")
			time.Sleep(2 * time.Second)

			err := i.ProcessCommits()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}

			i.moveToState(STATE_PROCESS_Complaints)
		case STATE_PROCESS_Complaints:
			err := i.ProcessComplaints()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			i.moveToState(STATE_PROCESS_ReconstructCommits)
		case STATE_PROCESS_ReconstructCommits:
			err := i.ProcessComplaints()
			if err != nil {
				//todo errcheck
				i.moveToState(STATE_PUBKEY_SEND)
				panic(err)
			}
			fmt.Println("DKG finished:", i.DkgRabin.Finished())
			return nil

		default:

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
