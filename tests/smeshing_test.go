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

func TestSmeshing(t *testing.T) {
	t.Parallel()
	const (
		limit = 15
	)

	cctx, err := clustercontext.New(t)
	require.NoError(t, err)

	cl := cluster.New(
		cluster.WithSmesherImage(cctx.Image),
		cluster.WithGenesisTime(time.Now().Add(cctx.BootstrapDuration)),
		cluster.WithTargetOutbound(defaultTargetOutbound(cctx.ClusterSize)),
	)
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddSmeshers(cctx, cctx.ClusterSize-2))

	createdch := make(chan *spacemeshv1.Proposal, cl.Total()*limit)
	includedAll := make([]map[uint32][]*spacemeshv1.Proposal, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		includedAll[i] = map[uint32][]*spacemeshv1.Proposal{}
	}

	eg, ctx := errgroup.WithContext(cctx)
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectProposals(ctx, eg, cl.Client(i), func(proposal *spacemeshv1.Proposal) bool {
			cctx.Log.Debugw("received proposal event",
				"client", client.Name,
				"layer", proposal.Layer.Number,
				"smesher", prettyHex(proposal.Smesher.Id),
				"eligibilities", len(proposal.Eligibilities),
				"status", spacemeshv1.Proposal_Status_name[int32(proposal.Status)],
			)
			if proposal.Layer.Number > limit {
				return false
			}
			if proposal.Status == spacemeshv1.Proposal_Created {
				createdch <- proposal
			} else {
				includedAll[i][proposal.Layer.Number] = append(includedAll[i][proposal.Layer.Number], proposal)
			}
			return true
		})
	}

	require.NoError(t, eg.Wait())
	close(createdch)

	created := map[uint32][]*spacemeshv1.Proposal{}
	for proposal := range createdch {
		created[proposal.Layer.Number] = append(created[proposal.Layer.Number], proposal)
	}
	requireEqualEligibilities(t, created)
	for layer := range created {
		sort.Slice(created[layer], func(i, j int) bool {
			return bytes.Compare(created[layer][i].Smesher.Id, created[layer][i].Smesher.Id) == -1
		})
	}
	for _, included := range includedAll {
		for layer := range included {
			sort.Slice(included[layer], func(i, j int) bool {
				return bytes.Compare(included[layer][i].Smesher.Id, included[layer][i].Smesher.Id) == -1
			})
		}
		for layer, proposals := range created {
			require.Len(t, included[layer], len(proposals))
		}
	}
}

func prettyHex(buf []byte) string {
	return fmt.Sprintf("0x%x", buf)
}

func requireEqualEligibilities(tb testing.TB, proposals map[uint32][]*spacemeshv1.Proposal) {
	tb.Helper()

	aggregated := map[string]int{}
	for _, perlayer := range proposals {
		for _, proposal := range perlayer {
			aggregated[string(proposal.Smesher.Id)] += len(proposal.Eligibilities)
		}
	}
	referenceEligibilities := -1
	for smesher, eligibilities := range aggregated {
		if referenceEligibilities < 0 {
			referenceEligibilities = eligibilities
		} else {
			require.Equal(tb, referenceEligibilities, eligibilities, smesher)
		}
	}
}

func collectProposals(ctx context.Context, eg *errgroup.Group, client *cluster.NodeClient, collector func(*spacemeshv1.Proposal) bool) {
	eg.Go(func() error {
		dbg := spacemeshv1.NewDebugServiceClient(client)
		proposals, err := dbg.ProposalsStream(ctx, &empty.Empty{})
		if err != nil {
			return err
		}
		for {
			proposal, err := proposals.Recv()
			if err != nil {
				return err
			}
			if !collector(proposal) {
				return nil
			}
		}
	})
}
