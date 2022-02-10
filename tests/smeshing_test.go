package tests

import (
	"bytes"
	"sort"
	"testing"

	"github.com/dshulyak/systest/cluster"
	ccontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestSmeshing(t *testing.T) {
	const limit = 15

	cctx := ccontext.Init(t, ccontext.Labels("sanity"))
	cl, err := cluster.Default(cctx)
	require.NoError(t, err)

	createdch := make(chan *spacemeshv1.Proposal, cl.Total()*limit)
	includedAll := make([]map[uint32][]*spacemeshv1.Proposal, cl.Total())
	for i := 0; i < cl.Total(); i++ {
		includedAll[i] = map[uint32][]*spacemeshv1.Proposal{}
	}

	eg, ctx := errgroup.WithContext(cctx)
	for i := 0; i < cl.Total(); i++ {
		i := i
		client := cl.Client(i)
		collectProposals(ctx, eg, cl.Client(i), func(proposal *spacemeshv1.Proposal) (bool, error) {
			cctx.Log.Debugw("received proposal event",
				"client", client.Name,
				"layer", proposal.Layer.Number,
				"smesher", prettyHex(proposal.Smesher.Id),
				"eligibilities", len(proposal.Eligibilities),
				"status", spacemeshv1.Proposal_Status_name[int32(proposal.Status)],
			)
			if proposal.Layer.Number > limit {
				return false, nil
			}
			if proposal.Status == spacemeshv1.Proposal_Created {
				createdch <- proposal
			} else {
				includedAll[i][proposal.Layer.Number] = append(includedAll[i][proposal.Layer.Number], proposal)
			}
			return true, nil
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
			return bytes.Compare(created[layer][i].Smesher.Id, created[layer][j].Smesher.Id) == -1
		})
	}
	for _, included := range includedAll {
		for layer := range included {
			sort.Slice(included[layer], func(i, j int) bool {
				return bytes.Compare(included[layer][i].Smesher.Id, included[layer][j].Smesher.Id) == -1
			})
		}
		for layer, proposals := range created {
			require.Len(t, included[layer], len(proposals))
			for i := range proposals {
				require.Equal(t, proposals[i].Id, included[layer][i].Id,
					"layer=%d client=%s", layer, cl.Client(i).Name)
			}
		}
	}
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
