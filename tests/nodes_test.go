package tests

import (
	"strings"
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

func TestAddNodes(t *testing.T) {
	const (
		beforeAdding = 11
		// 4 epochs to fully join:
		// sync finishes at layer 16
		// atx published layer 20, in the next epoch node will participate in beacon
		// after beacon computed - node will builder proposals
		fullyJoined = beforeAdding + 16
		lastLayer   = fullyJoined + 8
	)

	cctx := ccontext.Init(t, ccontext.Labels("sanity"))
	addedLater := int(0.2 * float64(cctx.ClusterSize))

	cl := cluster.New(
		cluster.WithSmesherImage(cctx.Image),
		cluster.WithGenesisTime(time.Now().Add(cctx.BootstrapDuration)),
		cluster.WithTargetOutbound(defaultTargetOutbound(cctx.ClusterSize)),
	)
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddSmeshers(cctx, cctx.ClusterSize-2-addedLater))

	var eg errgroup.Group
	{
		collectLayers(cctx, &eg, cl.Client(0), func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
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
		collectProposals(cctx, &eg, cl.Client(i), func(proposal *spacemeshv1.Proposal) bool {
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

func TestFailedNodes(t *testing.T) {
	const (
		failAt    = 15
		lastLayer = failAt + 16
	)

	cctx := ccontext.Init(t, ccontext.Labels("sanity"))
	cl, err := cluster.Default(cctx)
	require.NoError(t, err)

	failed := int(0.6 * float64(cctx.ClusterSize))

	eg, ctx := errgroup.WithContext(cctx)
	{
		var (
			teardown chaos.Teardown
			err      error
		)
		collectLayers(ctx, eg, cl.Client(0), func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Number.Number == failAt && teardown == nil {
				names := []string{}
				for i := 1; i <= failed; i++ {
					names = append(names, cl.Client(cl.Total()-i).Name)
				}
				cctx.Log.Debugw("failing nodes", "names", strings.Join(names, ","))
				err, teardown = chaos.Fail(cctx, "fail60percent", names...)
				if err != nil {
					return false, err
				}
				return false, nil
			}
			return true, nil
		})
	}
	hashes := make([]map[uint32]string, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		hashes[i] = map[uint32]string{}
	}
	for i := 0; i < cl.Total()-failed; i++ {
		i := i
		client := cl.Client(i)
		collectLayers(ctx, eg, client, func(layer *spacemeshv1.LayerStreamResponse) (bool, error) {
			if layer.Layer.Status == spacemeshv1.Layer_LAYER_STATUS_CONFIRMED {
				cctx.Log.Debugw("confirmed layer",
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
}
