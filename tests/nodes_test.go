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

func TestAddNodes(t *testing.T) {
	t.Parallel()
	const (
		beforeAdding = 11
		// 4 epochs to fully join:
		// sync finishes at layer 16
		// atx published layer 20, in the next epoch node will participate in beacon
		// after beacon computed - node will builder proposals
		fullyJoined = beforeAdding + 16
		lastLayer   = fullyJoined + 8
		timeout     = 10 * time.Minute
	)

	cctx, err := clustercontext.New(t)
	require.NoError(t, err)
	addedLater := int(0.2 * float64(cctx.ClusterSize))

	cl := cluster.New(
		cluster.WithSmesherImage(cctx.Image),
		cluster.WithGenesisTime(time.Now().Add(cctx.BootstrapDuration)),
		cluster.WithTargetOutbound(defaultTargetOutbound(cctx.ClusterSize)),
	)
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddSmeshers(cctx, cctx.ClusterSize-2-addedLater))

	var eg errgroup.Group
	ctx, cancel := context.WithTimeout(cctx, timeout)
	defer cancel()
	{
		collectLayers(ctx, &eg, cl.Client(0), func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Number.Number >= beforeAdding {
				cctx.Log.Debugw("adding new smeshers",
					"n", addedLater,
					"layer", layer.Layer.Number,
				)
				return false, cl.AddSmeshers(cctx, addedLater)
			}
			return true, nil
		})
	}
	require.NoError(t, eg.Wait())

	created := make([][]*spacemeshv1.Proposal, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectProposals(ctx, &eg, cl.Client(i), func(proposal *spacemeshv1.Proposal) bool {
			if proposal.Layer.Number > lastLayer {
				return false
			}
			if proposal.Status == spacemeshv1.Proposal_Created {
				cctx.Log.Debugw("received proposal event",
					"client", client.Name,
					"layer", proposal.Layer.Number,
					"smesher", prettyHex(proposal.Smesher.Id),
					"eligibilities", len(proposal.Eligibilities),
					"status", spacemeshv1.Proposal_Status_name[int32(proposal.Status)],
				)
				created[i] = append(created[i], proposal)
			}
			return true
		})
	}
	require.NoError(t, eg.Wait())
	// TODO unique by epoch, not layer
	// test eligibilities equality
	unique := map[uint32]map[string]struct{}{}
	eligibilities := map[uint32]int{}
	for _, proposals := range created {
		for _, proposal := range proposals {
			if _, exist := unique[proposal.Layer.Number]; !exist {
				unique[proposal.Layer.Number] = map[string]struct{}{}
			}
			unique[proposal.Layer.Number][prettyHex(proposal.Smesher.Id)] = struct{}{}
			eligibilities[proposal.Layer.Number] += len(proposal.Eligibilities)
		}
	}
	for layer := uint32(beforeAdding) + 1; layer <= fullyJoined; layer++ {
		require.Len(t, unique[layer], cl.Total()-addedLater, "layer=%d", layer)
	}
	for layer := uint32(fullyJoined) + 1; layer <= lastLayer; layer++ {
		require.Len(t, unique[layer], cl.Total(), "layer=%d", layer)
	}
}
