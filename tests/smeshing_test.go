package tests

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"
	"github.com/golang/protobuf/ptypes/empty"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type rewardsResult struct {
	layers  []uint32
	address []byte
	sum     uint64
}

// func defaultBootnodeCount(size int) int {
// 	if size < 10 {
// 		return 1
// 	}
// 	return int(0.1 * float64(size))
// }

func TestSmeshing(t *testing.T) {
	t.Parallel()
	const (
		limit   = 15
		timeout = 10 * time.Minute // > 19 layers + bootstrap time
	)

	cctx, err := clustercontext.New(t)
	require.NoError(t, err)

	cl := cluster.New(
		cluster.WithSmesherImage(cctx.Image),
		cluster.WithGenesisTime(time.Now().Add(cctx.BootstrapDuration)),
		cluster.WithTargetOutbound(defaultTargetOutbound(cctx.ClusterSize)),
	)
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddSmeshers(cctx, cctx.ClusterSize-2))

	createdch := make(chan *spacemeshv1.Proposal, cl.Total()*limit)
	includedch := make(chan map[uint32][]*spacemeshv1.Proposal, cl.Total())

	eg, ctx := errgroup.WithContext(cctx)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		dbg := spacemeshv1.NewDebugServiceClient(client)
		eg.Go(func() error {
			proposals, err := dbg.ProposalsStream(ctx, &empty.Empty{})
			if err != nil {
				return err
			}
			included := map[uint32][]*spacemeshv1.Proposal{}
			for {
				proposal, err := proposals.Recv()
				if err != nil {
					return err
				}
				if proposal.Layer.Number > limit {
					break
				}
				cctx.Log.Debugw("received proposal event",
					"client", i,
					"layer", proposal.Layer.Number,
					"smesher", prettyString(proposal.Smesher.Id),
					"eligibilities", len(proposal.Eligibilities),
					"status", spacemeshv1.Proposal_Status_name[int32(proposal.Status)],
				)
				if proposal.Status == spacemeshv1.Proposal_Created {
					createdch <- proposal
				} else {
					included[proposal.Layer.Number] = append(included[proposal.Layer.Number], proposal)
				}
			}
			for layer := range included {
				sort.Slice(included[layer], func(i, j int) bool {
					return bytes.Compare(included[layer][i].Smesher.Id, included[layer][j].Smesher.Id) == -1
				})
			}
			includedch <- included
			return nil
		})
	}

	require.NoError(t, eg.Wait())
	close(createdch)
	close(includedch)

	aggregated := map[string]int{}
	created := map[uint32][]*spacemeshv1.Proposal{}
	for proposal := range createdch {
		created[proposal.Layer.Number] = append(created[proposal.Layer.Number], proposal)
		aggregated[prettyString(proposal.Smesher.Id)] += len(proposal.Eligibilities)
	}
	referenceEligibilities := -1
	for smesher, eligibilities := range aggregated {
		if referenceEligibilities < 0 {
			referenceEligibilities = eligibilities
		} else {
			require.Equal(t, referenceEligibilities, eligibilities, smesher)
		}
	}

	for layer := range created {
		sort.Slice(created[layer], func(i, j int) bool {
			return bytes.Compare(created[layer][i].Smesher.Id, created[layer][i].Smesher.Id) == -1
		})
	}
	for included := range includedch {
		for layer, proposals := range created {
			require.Len(t, included[layer], len(proposals))
			require.Equal(t, included[layer], proposals)
		}
	}
}

func prettyString(buf []byte) string {
	return fmt.Sprintf("0x%x", buf)
}
