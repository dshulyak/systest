package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"
	"google.golang.org/protobuf/types/known/emptypb"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/stretchr/testify/require"
)

func TestSmeshing(t *testing.T) {
	cctx, err := clustercontext.New(context.Background(), t)
	require.NoError(t, err)

	cl := cluster.New(cluster.WithSmesherImage(cctx.Image))
	require.NoError(t, cl.AddPoet(cctx))
	require.NoError(t, cl.AddBootnodes(cctx, 2))
	require.NoError(t, cl.AddSmeshers(cctx, 4))
	t.Log("deployment completed")
	smesherapi := spacemeshv1.NewSmesherServiceClient(cl.Client(0))
	stateapi := spacemeshv1.NewGlobalStateServiceClient(cl.Client(2))
	id, err := smesherapi.SmesherID(cctx, &emptypb.Empty{})
	require.NoError(t, err)
	rewards, err := stateapi.SmesherRewardStream(cctx, &spacemeshv1.SmesherRewardStreamRequest{
		Id: &spacemeshv1.SmesherId{Id: id.AccountId.Address},
	})
	require.NoError(t, err)
	for {
		reward, err := rewards.Recv()
		require.NoError(t, err)
		fmt.Printf("%d: 0x%x = %d\n", reward.Reward.Layer.Number, reward.Reward.Smesher.Id, reward.Reward.LayerReward.Value)
	}
}
