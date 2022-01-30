package tests

import (
	"context"
	"testing"

	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rewardsResult struct {
	layers  []uint32
	address []byte
	sum     uint64
}

func TestSmeshing(t *testing.T) {
	const (
		smeshers = 10
		layers   = 16 // multiple of 4, epoch is 4 layers
	)

	cctx, err := clustercontext.New(context.Background(), t)
	require.NoError(t, err)

	cl := cluster.New(cluster.WithSmesherImage(cctx.Image))
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddSmeshers(cctx, smeshers-2))

	results := make(chan rewardsResult, smeshers)
	eg, ctx := errgroup.WithContext(cctx)
	for i := 0; i < smeshers; i++ {
		smesherapi := spacemeshv1.NewSmesherServiceClient(cl.Client(i))
		stateapi := spacemeshv1.NewGlobalStateServiceClient(cl.Client(i))
		eg.Go(func() error {
			id, err := smesherapi.SmesherID(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			rewards, err := stateapi.SmesherRewardStream(ctx, &spacemeshv1.SmesherRewardStreamRequest{
				Id: &spacemeshv1.SmesherId{Id: id.AccountId.Address},
			})
			if err != nil {
				return err
			}
			rst := rewardsResult{address: id.AccountId.Address}
			for l := 0; l < layers; l++ {
				reward, err := rewards.Recv()
				if err != nil {
					return err
				}
				rst.layers = append(rst.layers, reward.Reward.Layer.Number)
				rst.sum += reward.Reward.LayerReward.Value
				t.Logf("%d: 0x%x => %d\n", reward.Reward.Layer.Number, rst.address, rst.sum)
			}
			results <- rst
			return nil
		})
	}
	require.NoError(t, eg.Wait())
	close(results)
	var reference *rewardsResult
	for tested := range results {
		if reference == nil {
			reference = &tested
		} else {
			// are they not equal because of the cluster size?
			assert.InDelta(t, reference.sum, tested.sum, float64(reference.sum)*0.1,
				"reference=0x%x != tested=0x%x", tested.address,
			)
		}
	}
}
