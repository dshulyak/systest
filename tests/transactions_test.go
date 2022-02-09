package tests

import (
	"context"
	"testing"
	"time"

	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestTransactions(t *testing.T) {
	t.Parallel()
	const (
		keys        = 10
		stopSending = 14
		stopWaiting = 16
		batch       = 5
		amount      = 100
	)
	receiver := [20]byte{11, 1, 1}

	cctx, err := clustercontext.New(t)
	require.NoError(t, err)

	cl := cluster.New(
		cluster.WithSmesherImage(cctx.Image),
		cluster.WithGenesisTime(time.Now().Add(cctx.BootstrapDuration)),
		cluster.WithTargetOutbound(defaultTargetOutbound(cctx.ClusterSize)),
		cluster.WithKeys(keys),
	)
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddSmeshers(cctx, cctx.ClusterSize-2))

	eg, ctx := errgroup.WithContext(cctx)
	for i := 0; i < keys; i++ {
		client := cl.Client(i % cl.Total())
		meshapi := spacemeshv1.NewMeshServiceClient(client)
		private := cl.Private(i)
		eg.Go(func() error {
			layers, err := meshapi.LayerStream(cctx, &spacemeshv1.LayerStreamRequest{})
			if err != nil {
				return err
			}
			var (
				nonce    uint64
				maxLayer uint32
			)
			for {
				layer, err := layers.Recv()
				if err != nil {
					return err
				}
				if layer.Layer.Status != spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
					continue
				}
				if layer.Layer.Number.Number <= maxLayer {
					continue
				}
				maxLayer = layer.Layer.Number.Number
				if layer.Layer.Number.Number == stopSending {
					return nil
				}
				for j := 0; j < batch; j++ {
					cctx.Log.Debugw("submitting transactions",
						"layer", layer.Layer.Number,
						"client", client.Name,
						"nonce", nonce,
						"batch", batch,
					)
					if err := submitTransacition(ctx, private, transaction{
						GasLimit:  100,
						Fee:       1,
						Amount:    amount,
						Recipient: receiver,
						Nonce:     nonce,
					}, client); err != nil {
						return err
					}
					nonce++
				}
			}
		})
	}
	results := make(chan []*spacemeshv1.Transaction, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		client := cl.Client(i)
		meshapi := spacemeshv1.NewMeshServiceClient(client)
		eg.Go(func() error {
			sctx, cancel := context.WithCancel(ctx)
			defer cancel()
			layers, err := meshapi.LayerStream(sctx, &spacemeshv1.LayerStreamRequest{})
			if err != nil {
				return err
			}
			var txs []*spacemeshv1.Transaction
			for {
				layer, err := layers.Recv()
				if err != nil {
					return err
				}
				if layer.Layer.Status != spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
					continue
				}
				if layer.Layer.Number.Number == stopWaiting {
					break
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
				txs = append(txs, addtxs...)
			}
			results <- txs
			return nil
		})
	}
	require.NoError(t, eg.Wait())
	close(results)
	var reference []*spacemeshv1.Transaction
	for tested := range results {
		if reference == nil {
			reference = tested
			require.NotEmpty(t, reference)
		} else {
			require.Len(t, tested, len(reference))
			for i := range reference {
				require.Equal(t, reference[i], tested[i])
			}
		}
	}
}
