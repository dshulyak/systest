package tests

import (
	"testing"

	"github.com/dshulyak/systest/cluster"
	"github.com/dshulyak/systest/testcontext"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestTransactions(t *testing.T) {
	cctx := testcontext.New(t, testcontext.Labels("sanity"))

	const (
		keys        = 10
		stopSending = 14
		stopWaiting = 16
		batch       = 5
		amount      = 100
	)
	receiver := [20]byte{11, 1, 1}

	cl, err := cluster.Default(cctx, cluster.WithKeys(keys))
	require.NoError(t, err)

	eg, ctx := errgroup.WithContext(cctx)
	for i := 0; i < keys; i++ {
		client := cl.Client(i % cl.Total())
		submitter := newTransactionSubmitter(cl.Private(i), receiver, amount, client)
		collectLayers(ctx, eg, client, func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Status != spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
				return true, nil
			}
			if layer.Layer.Number.Number == stopSending {
				return false, nil
			}
			cctx.Log.Debugw("submitting transactions",
				"layer", layer.Layer.Number,
				"client", client.Name,
				"batch", batch,
			)
			for j := 0; j < batch; j++ {
				if err := submitter(ctx); err != nil {
					return false, err
				}
			}
			return true, nil
		})
	}
	txs := make([][]*spacemeshv1.Transaction, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectLayers(ctx, eg, client, func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Status != spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
				return true, nil
			}
			if layer.Layer.Number.Number == stopWaiting {
				return false, nil
			}
			addtxs := []*spacemeshv1.Transaction{}
			for _, block := range layer.Layer.Blocks {
				addtxs = append(addtxs, block.Transactions...)
			}
			cctx.Log.Debugw("received transactions",
				"layer", layer.Layer.Number,
				"client", client.Name,
				"blocks", len(layer.Layer.Blocks),
				"transactions", len(addtxs),
			)
			txs[i] = append(txs[i], addtxs...)
			return true, nil
		})
	}

	require.NoError(t, eg.Wait())
	reference := txs[0]
	for _, tested := range txs[1:] {
		require.Len(t, tested, len(reference))
		for i := range reference {
			require.Equal(t, reference[i], tested[i])
		}
	}
}
