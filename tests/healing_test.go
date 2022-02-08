package tests

import (
	"context"
	"testing"
	"time"

	"github.com/dshulyak/systest/chaos"
	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"
	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestHealing(t *testing.T) {
	t.Parallel()
	const (
		smeshers  = 7
		partition = 12
		restore   = 17
		wait      = 60 // > 4minutes. 15s per layer
		timeout   = 10 * time.Minute
	)

	cctx, err := clustercontext.New(t)
	require.NoError(t, err)

	cl := cluster.New(
		cluster.WithSmesherImage(cctx.Image),
		cluster.WithGenesisTime(time.Now().Add(cctx.BootstrapDuration)),
		cluster.WithTargetOutbound(defaultTargetOutbound(cctx.ClusterSize)),
		cluster.WithRerunInterval(2*time.Minute),
	)
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddSmeshers(cctx, smeshers-2))

	hashes := make([]map[uint32][]byte, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		hashes[i] = map[uint32][]byte{}
	}
	eg, ctx := errgroup.WithContext(cctx)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	{
		var teardown chaos.Teardown
		collectLayers(ctx, eg, cl.Client(0), func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Number.Number >= partition && teardown == nil {
				err, teardown = chaos.Partition2(cctx, "partition5from2",
					extractNames(cl.Boot(0), cl.Smesher(0), cl.Smesher(1), cl.Smesher(2), cl.Smesher(3)),
					extractNames(cl.Boot(1), cl.Smesher(4)),
				)
				if err != nil {
					return false, err
				}
			}
			if layer.Layer.Number.Number >= restore {
				if err := teardown(ctx); err != nil {
					return false, err
				}
				return false, nil
			}
			return true, nil
		})
	}
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectLayers(ctx, eg, client, func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Status == spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
				cctx.Log.Debugw("confirmed layer",
					"client", client.Name,
					"layer", layer.Layer.Number.Number,
					"hash", layer.Layer.Hash,
				)
			}
			if layer.Layer.Number.Number == wait {
				return false, nil
			}
			hashes[i][layer.Layer.Number.Number] = layer.Layer.Hash
			return true, nil
		})
	}
	require.NoError(t, eg.Wait())
	reference := hashes[0]
	for _, tested := range hashes[1] {
		assert.Equal(t, reference, tested)
	}
}

func collectLayers(ctx context.Context, eg *errgroup.Group, client *cluster.NodeClient,
	collector func(*spacemeshv1.LayerStreamResponse) (bool, error)) {
	eg.Go(func() error {
		meshapi := spacemeshv1.NewMeshServiceClient(client)
		layers, err := meshapi.LayerStream(ctx, &spacemeshv1.LayerStreamRequest{})
		if err != nil {
			return err
		}
		for {
			layer, err := layers.Recv()
			if err != nil {
				return err
			}
			if cont, err := collector(layer); !cont {
				return err
			}
		}
	})
}
