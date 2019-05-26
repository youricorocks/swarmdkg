package swarmdkg

import (
	"fmt"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/swarm/storage/feed"
)

var counter = new(int64)
const startValue = 48879 //hex 'beef'

func init() {
	*counter = startValue
}

func newTestSigner() (*feed.GenericSigner, error) {
	tailBytes := fmt.Sprintf("%04x", atomic.AddInt64(counter, 1)-1)

	privKey, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdead" + tailBytes)
	if err != nil {
		return nil, err
	}
	return feed.NewGenericSigner(privKey), nil
}

func newTestSigners(n int) ([]*feed.GenericSigner, error) {
	counter := 0

	signers := make([]*feed.GenericSigner, n)

	for i := range signers {
		tailBytes := fmt.Sprintf("%04x", counter)
		privKey, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdead" + tailBytes)
		if err != nil {
			return nil, err
		}

		signers[i] = feed.NewGenericSigner(privKey)
		counter++
	}

	return signers, nil
}
