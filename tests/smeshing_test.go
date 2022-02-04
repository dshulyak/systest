package tests

import (
	"testing"
	"time"

	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"
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
		layers   = 16 // multiple of 4, epoch is 4 layers
		maxLayer = 23 // genesis + 16
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

	results, err := collectRewards(cctx, cl, maxLayer)
	require.NoError(t, err)
	close(results)
	var reference *rewardsResult
	for tested := range results {
		if reference == nil {
			reference = &tested
		} else {
			// are they not equal because of the cluster size?
			assert.InDelta(t, reference.sum, tested.sum, float64(reference.sum)*0.1,
				"reference=0x%x != tested=0x%x", reference.address, tested.address,
			)
		}
	}
}

func collectRewards(cctx *clustercontext.Context, cl *cluster.Cluster, upto uint32) (chan rewardsResult, error) {
	results := make(chan rewardsResult, cl.Total())
	eg, ctx := errgroup.WithContext(cctx)
	for i := 0; i < cl.Total(); i++ {
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
			for {
				reward, err := rewards.Recv()
				if err != nil {
					return err
				}
				if reward.Reward.Layer.Number > upto {
					break
				}
				rst.layers = append(rst.layers, reward.Reward.Layer.Number)
				rst.sum += reward.Reward.LayerReward.Value
				cctx.Log.Debugf("%d: 0x%x => %d\n", reward.Reward.Layer.Number, rst.address, rst.sum)
			}
			results <- rst
			return nil
		})
	}
	return results, eg.Wait()
}