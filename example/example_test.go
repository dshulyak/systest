package example

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dshulyak/systest/chaos"
	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"

	"github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
	ctx, err := clustercontext.New(context.Background())
	require.NoError(t, err)
	t.Logf("using namespace. ns=%s", ctx.Namespace)

	bootconf := cluster.DeployConfig{
		Name:     "boot",
		Headless: "boot-headless",
		Count:    2,
		Image:    "spacemeshos/go-spacemesh-dev:expose-network-identity",
	}

	if err := cluster.DeployNamespace(ctx); err != nil {
		require.NoError(t, err)
	}
	poet, err := cluster.DeployPoet(ctx,
		fmt.Sprintf("dns:///%s-0.%s:9092", bootconf.Name, bootconf.Headless),
		fmt.Sprintf("dns:///%s-1.%s:9092", bootconf.Name, bootconf.Headless),
	)
	if err != nil {
		require.NoError(t, err)
	}
	t.Logf("using poet endpoint. endpoint=%s", poet)

	smconf := cluster.SMConfig{
		GenesisTime:  time.Now().Add(1 * time.Minute),
		NetworkID:    777,
		PoetEndpoint: poet,
	}
	bootnodes, err := cluster.DeployNodes(ctx, bootconf, smconf)
	if err != nil {
		require.NoError(t, err)
	}
	for _, node := range bootnodes {
		smconf.Bootnodes = append(smconf.Bootnodes, node.P2PEndpoint())
	}
	t.Logf("using bootnodes %s", strings.Join(smconf.Bootnodes, ", "))

	nodesconf := bootconf
	nodesconf.Count = 4
	nodesconf.Headless = "smesher-headless"
	nodesconf.Name = "smesher"
	nodes, err := cluster.DeployNodes(ctx, nodesconf, smconf)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Minute)
		err, teardown := chaos.Partition2(ctx,
			getNames(bootnodes[0], nodes[0], nodes[1], nodes[2]),
			getNames(bootnodes[1], nodes[3]),
		)
		require.NoError(t, err)
		time.Sleep(30 * time.Minute)
		require.NoError(t, teardown(ctx))
	}
}

func getNames(nodes ...*cluster.NodeClient) []string {
	var rst []string
	for _, n := range nodes {
		rst = append(rst, n.Name)
	}
	return rst
}
