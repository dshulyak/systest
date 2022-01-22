package example

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
	_, err = cluster.DeployNodes(ctx, nodesconf, smconf)
	if err != nil {
		require.NoError(t, err)
	}
}
