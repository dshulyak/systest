package tests

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/dshulyak/systest/chaos"
	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/spacemeshos/ed25519"
	"github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
	ctx, err := clustercontext.New(context.Background(), t)
	require.NoError(t, err)

	cl := cluster.New(
		cluster.WithSmesherImage(ctx.Image),
	)
	require.NoError(t, cl.AddPoet(ctx))
	require.NoError(t, cl.AddBootnodes(ctx, 2))
	require.NoError(t, cl.AddSmeshers(ctx, 4))
	t.Log("deployment completed")
	go func() {
		tx := transaction{
			GasLimit:  100,
			Fee:       1,
			Amount:    1,
			Recipient: [20]byte{1, 1, 1, 1},
		}
		ticker := time.NewTicker(2 * time.Minute)
		for {
			select {
			case <-ticker.C:
				submitTransacition(t, ctx, cl.Private(0), tx, cl.Client(0))
				tx.Nonce++
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		time.Sleep(40 * time.Minute)
		t.Log("enabling partition")
		err, teardown := chaos.Partition2(ctx, "partition4from2",
			extractNames(cl.Boot(0), cl.Smesher(0), cl.Smesher(1), cl.Smesher(2)),
			extractNames(cl.Boot(1), cl.Smesher(3)),
		)
		require.NoError(t, err)
		time.Sleep(20 * time.Minute)
		require.NoError(t, teardown(ctx))
		t.Log("partition removed")
	}
}

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

func submitTransacition(tb testing.TB, ctx context.Context, pk ed25519.PrivateKey, tx transaction, node *cluster.NodeClient) {
	txclient := spacemeshv1.NewTransactionServiceClient(node.Conn)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	encoded := encodeTx(tx)
	encoded = append(encoded, ed25519.Sign2(pk, encoded)...)
	response, err := txclient.SubmitTransaction(ctx, &spacemeshv1.SubmitTransactionRequest{Transaction: encoded})
	require.NoError(tb, err)
	require.NotNil(tb, response.Txstate, "tx state is nil")
}

func extractNames(nodes ...*cluster.NodeClient) []string {
	var rst []string
	for _, n := range nodes {
		rst = append(rst, n.Name)
	}
	return rst
}
