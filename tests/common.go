package tests

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/dshulyak/systest/cluster"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/spacemeshos/ed25519"
)

type transaction struct {
	Nonce     uint64
	Recipient [20]byte
	GasLimit  uint64
	Fee       uint64
	Amount    uint64
}

func encodeTx(tx transaction) (buf []byte) {
	scratch := [8]byte{}
	binary.BigEndian.PutUint64(scratch[:], tx.Nonce)
	buf = append(buf, scratch[:]...)
	buf = append(buf, tx.Recipient[:]...)
	binary.BigEndian.PutUint64(scratch[:], tx.GasLimit)
	buf = append(buf, scratch[:]...)
	binary.BigEndian.PutUint64(scratch[:], tx.Fee)
	buf = append(buf, scratch[:]...)
	binary.BigEndian.PutUint64(scratch[:], tx.Amount)
	buf = append(buf, scratch[:]...)
	return buf
}

func submitTransacition(ctx context.Context, pk ed25519.PrivateKey, tx transaction, node *cluster.NodeClient) error {
	txclient := spacemeshv1.NewTransactionServiceClient(node)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	encoded := encodeTx(tx)
	encoded = append(encoded, ed25519.Sign2(pk, encoded)...)
	response, err := txclient.SubmitTransaction(ctx, &spacemeshv1.SubmitTransactionRequest{Transaction: encoded})
	if err != nil {
		return err
	}
	if response.Txstate == nil {
		return fmt.Errorf("tx state should not be nil")
	}
	return nil
}

func extractNames(nodes ...*cluster.NodeClient) []string {
	var rst []string
	for _, n := range nodes {
		rst = append(rst, n.Name)
	}
	return rst
}

func defaultTargetOutbound(size int) int {
	if size < 10 {
		return 3
	}
	return int(0.3 * float64(size))
}
