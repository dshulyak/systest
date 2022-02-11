package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/dshulyak/systest/chaos"
	"github.com/dshulyak/systest/cluster"
	"github.com/dshulyak/systest/testcontext"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestAddNodes(t *testing.T) {
	tctx := testcontext.New(t, testcontext.Labels("sanity"))

	const (
		epochBeforeJoin = 5
		lastEpoch       = 7

		beforeAdding = 11
		// 4 epochs to fully join:
		// sync finishes at layer 16
		// atx published layer 20, in the next epoch node will participate in beacon
		// after beacon computed - node will build proposals
		fullyJoined = beforeAdding + 16
		lastLayer   = fullyJoined + 8
	)

	cl := cluster.New(tctx)

	require.NoError(t, cl.AddBootnodes(tctx, 2))
	require.NoError(t, cl.AddPoet(tctx))
	addedLater := int(0.2 * float64(tctx.ClusterSize))
	require.NoError(t, cl.AddSmeshers(tctx, tctx.ClusterSize-2-addedLater))

	var eg errgroup.Group
	{
		collectLayers(tctx, &eg, cl.Client(0), func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Number.Number >= beforeAdding {
				tctx.Log.Debugw("adding new smeshers",
					"n", addedLater,
					"layer", layer.Layer.Number,
				)
				return false, cl.AddSmeshers(tctx, addedLater)
			}
			return true, nil
		})
	}
	require.NoError(t, eg.Wait())

	created := make([][]*spacemeshv1.Proposal, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectProposals(tctx, &eg, cl.Client(i), func(proposal *spacemeshv1.Proposal) (bool, error) {
			if proposal.Layer.Number > lastLayer {
				return false, nil
			}
			if proposal.Status == spacemeshv1.Proposal_Created {
				tctx.Log.Debugw("received proposal event",
					"client", client.Name,
					"layer", proposal.Layer.Number,
					"epoch", proposal.Epoch.Value,
					"smesher", prettyHex(proposal.Smesher.Id),
					"eligibilities", len(proposal.Eligibilities),
					"status", spacemeshv1.Proposal_Status_name[int32(proposal.Status)],
				)
				created[i] = append(created[i], proposal)
			}
			return true, nil
		})
	}
	require.NoError(t, eg.Wait())
	unique := map[uint64]map[string]struct{}{}
	for _, proposals := range created {
		for _, proposal := range proposals {
			if _, exist := unique[proposal.Epoch.Value]; !exist {
				unique[proposal.Epoch.Value] = map[string]struct{}{}
			}
			unique[proposal.Epoch.Value][prettyHex(proposal.Smesher.Id)] = struct{}{}
		}
	}
	for epoch := uint64(2) + 1; epoch <= epochBeforeJoin; epoch++ {
		require.Len(t, unique[epoch], cl.Total()-addedLater, "epoch=%d", epoch)
	}
	for epoch := uint64(epochBeforeJoin) + 1; epoch <= lastEpoch; epoch++ {
		require.Len(t, unique[epoch], cl.Total(), "epoch=%d", epoch)
	}
}

func TestFailedNodes(t *testing.T) {
	tctx := testcontext.New(t, testcontext.Labels("sanity"))

	const (
		failAt    = 15
		lastLayer = failAt + 8
	)

	cl, err := cluster.Default(tctx)
	require.NoError(t, err)

	failed := int(0.6 * float64(tctx.ClusterSize))

	eg, ctx := errgroup.WithContext(tctx)
	scheduleChaos(ctx, eg, cl.Client(0), failAt, lastLayer, func(ctx context.Context) (error, chaos.Teardown) {
		names := []string{}
		for i := 1; i <= failed; i++ {
			names = append(names, cl.Client(cl.Total()-i).Name)
		}
		tctx.Log.Debugw("failing nodes", "names", strings.Join(names, ","))
		return chaos.Fail(tctx, "fail60percent", names...)
	})

	hashes := make([]map[uint32]string, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		hashes[i] = map[uint32]string{}
	}
	for i := 0; i < cl.Total()-failed; i++ {
		i := i
		client := cl.Client(i)
		collectLayers(ctx, eg, client, func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Status == spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
				tctx.Log.Debugw("confirmed layer",
					"client", client.Name,
					"layer", layer.Layer.Number.Number,
					"hash", prettyHex(layer.Layer.Hash),
				)
				if layer.Layer.Number.Number == lastLayer {
					return false, nil
				}
				hashes[i][layer.Layer.Number.Number] = prettyHex(layer.Layer.Hash)
			}
			return true, nil
		})
	}
	require.NoError(t, eg.Wait())
	reference := hashes[0]
	for i, tested := range hashes[1 : cl.Total()-failed] {
		assert.Equal(t, reference, tested, "client=%d", i)
	}
	require.NoError(t, waitAll(tctx, cl))
}
