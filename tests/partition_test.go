package tests

import (
	"context"
	"testing"
	"time"

	"github.com/dshulyak/systest/chaos"
	"github.com/dshulyak/systest/cluster"
	ccontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestPartition(t *testing.T) {
	const (
		smeshers  = 7
		partition = 13
		restore   = 20
		wait      = 50
	)

	cctx := ccontext.Init(t, ccontext.Labels("sanity"))

	cl, err := cluster.Default(cctx,
		cluster.WithSmesherFlag(cluster.RerunInterval(2*time.Minute)),
	)
	require.NoError(t, err)

	hashes := make([]map[uint32]string, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		hashes[i] = map[uint32]string{}
	}
	eg, ctx := errgroup.WithContext(cctx)

	scheduleChaos(ctx, eg, cl.Client(0), partition, restore, func(ctx context.Context) (error, chaos.Teardown) {
		return chaos.Partition2(cctx, "partition5from2",
			extractNames(cl.Boot(0), cl.Smesher(0), cl.Smesher(1), cl.Smesher(2), cl.Smesher(3)),
			extractNames(cl.Boot(1), cl.Smesher(4)),
		)
	})
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectLayers(ctx, eg, client, func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Status == spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
				cctx.Log.Debugw("confirmed layer",
					"client", client.Name,
					"layer", layer.Layer.Number.Number,
					"hash", prettyHex(layer.Layer.Hash),
				)
				if layer.Layer.Number.Number == wait {
					return false, nil
				}
				hashes[i][layer.Layer.Number.Number] = prettyHex(layer.Layer.Hash)
			}
			return true, nil
		})
	}
	require.NoError(t, eg.Wait())
	reference := hashes[0]
	for i, tested := range hashes[1:] {
		assert.Equal(t, reference, tested, "client=%s", cl.Client(i+1).Name)
	}
}
