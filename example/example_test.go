package example

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/dshulyak/systest/cluster"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func rngName() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	const choices = "qwertyuiopasdfghjklzxcvbnm"
	buf := make([]byte, 4)
	for i := range buf {
		buf[i] = choices[rng.Intn(len(choices))]
	}
	return string(buf)
}

func TestExample(t *testing.T) {
	config, err := rest.InClusterConfig()
	if err != nil {
		require.NoError(t, err)
	}
	ns := "test-" + rngName()
	t.Logf("using namespace. ns=%s", ns)

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		require.NoError(t, err)
	}

	bootconf := cluster.DeployConfig{
		Name:     "boot",
		Headless: "boot-headless",
		Count:    2,
		Image:    "spacemeshos/go-spacemesh-dev:expose-network-identity",
	}

	ctx := cluster.Context{
		Context:   context.Background(),
		Namespace: ns,
		Client:    clientset,
	}
	if err := cluster.DeployNamespace(&ctx); err != nil {
		require.NoError(t, err)
	}
	poet, err := cluster.DeployPoet(&ctx, fmt.Sprintf("dns:///%s-0.%s:9092", bootconf.Name, bootconf.Headless))
	if err != nil {
		require.NoError(t, err)
	}
	t.Logf("using poet endpoint. endpoint=%s", poet)

	smconf := cluster.SMConfig{
		GenesisTime:  time.Now().Add(1 * time.Minute),
		NetworkID:    777,
		PoetEndpoint: poet,
	}
	bootnodes, err := cluster.DeployNodes(&ctx, bootconf, smconf)
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
	_, err = cluster.DeployNodes(&ctx, nodesconf, smconf)
	if err != nil {
		require.NoError(t, err)
	}
}
