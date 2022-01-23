package example

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dshulyak/systest/chaos"
	"github.com/dshulyak/systest/cluster"
	clustercontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/spacemeshos/ed25519"
	"github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
	ctx, err := clustercontext.New(context.Background())
	require.NoError(t, err)

	if err := cluster.DeployNamespace(ctx); err != nil {
		require.NoError(t, err)
	}
	t.Logf("using namespace. ns=%s", ctx.Namespace)
	// TODO cleanup even if interrupted
	defer cluster.Cleanup(ctx)

	signers := genSigners(t, 10)

	bootconf := cluster.DeployConfig{
		Name:     "boot",
		Headless: "boot-headless",
		Count:    2,
		Image:    "spacemeshos/go-spacemesh-dev:cmd-genesis-accounts",
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
		Genesis:      genGenesis(signers),
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

	// TODO wait for nodes to be ready
	go func() {
		tx := transaction{
			GasLimit:  100,
			Fee:       1,
			Amount:    1,
			Recipient: [20]byte{1, 1, 1, 1},
		}
		ticker := time.NewTicker(2 * time.Minute)
		for {
			select {
			case <-ticker.C:
				submitTransacition(t, ctx, signers[0], tx, nodes[0])
				tx.Nonce++
			case <-ctx.Done():
				return
			}
		}
	}()
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Minute)
		err, teardown := chaos.Partition2(ctx, "partition4from2",
			getNames(bootnodes[0], nodes[0], nodes[1], nodes[2]),
			getNames(bootnodes[1], nodes[3]),
		)
		require.NoError(t, err)
		time.Sleep(30 * time.Minute)
		require.NoError(t, teardown(ctx))
	}
}

func genGenesis(signers []*signer) (rst map[string]uint64) {
	rst = map[string]uint64{}
	for _, sig := range signers {
		rst[sig.Address()] = 100000000000000000
	}
	return
}

type signer struct {
	Pub ed25519.PublicKey
	PK  ed25519.PrivateKey
}

func (s *signer) Address() string {
	encoded := hex.EncodeToString(s.Pub[12:])
	return "0x" + encoded
}

func genSigners(tb testing.TB, n int) (rst []*signer) {
	for i := 0; i < n; i++ {
		rst = append(rst, genSigner(tb))
	}
	return
}

func genSigner(tb testing.TB) *signer {
	pub, pk, err := ed25519.GenerateKey(nil)
	require.NoError(tb, err)
	return &signer{Pub: pub, PK: pk}
}

func getNames(nodes ...*cluster.NodeClient) []string {
	var rst []string
	for _, n := range nodes {
		rst = append(rst, n.Name)
	}
	return rst
}

type transaction struct {
	Nonce     uint64
	Recipient [20]byte
	GasLimit  uint64
	Fee       uint64
	Amount    uint64
}

func encodeTx(tx transaction) (buf []byte) {
	scratch := [8]byte{}
	binary.BigEndian.PutUint64(scratch[:], tx.Nonce)
	buf = append(buf, scratch[:]...)
	buf = append(buf, tx.Recipient[:]...)
	binary.BigEndian.PutUint64(scratch[:], tx.GasLimit)
	buf = append(buf, scratch[:]...)
	binary.BigEndian.PutUint64(scratch[:], tx.Fee)
	buf = append(buf, scratch[:]...)
	binary.BigEndian.PutUint64(scratch[:], tx.Amount)
	buf = append(buf, scratch[:]...)
	return buf
}

func submitTransacition(tb testing.TB, ctx context.Context, sig *signer, tx transaction, node *cluster.NodeClient) {
	txclient := spacemeshv1.NewTransactionServiceClient(node.Conn)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	encoded := encodeTx(tx)
	encoded = append(encoded, ed25519.Sign2(sig.PK, encoded)...)
	response, err := txclient.SubmitTransaction(ctx, &spacemeshv1.SubmitTransactionRequest{Transaction: encoded})
	require.NoError(tb, err)
	require.NotNil(tb, response.Txstate, "tx state is nil")
}
