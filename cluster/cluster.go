package cluster

import (
	"encoding/hex"
	"fmt"
	"time"

	clustercontext "github.com/dshulyak/systest/context"
	"github.com/spacemeshos/ed25519"
)

const (
	defaultNetID = 777
	poetSvc      = "poet"
	poetPort     = 80
	bootSvc      = "boot-headless"
)

func poetEndpoint() string {
	return fmt.Sprintf("%s:%d", poetSvc, poetPort)
}

// Opt is for configuring cluster.
type Opt func(c *Cluster)

// WithSmesherImage configures image for bootnodes and regular smesher nodes.
func WithSmesherImage(image string) Opt {
	return func(c *Cluster) {
		c.image = image
	}
}

// WithGenesisTime configures genesis time.
func WithGenesisTime(time time.Time) Opt {
	return func(c *Cluster) {
		c.genesisTime = time
	}
}

// WithTargetOutbound modifies number of connections each node will try to establish.
func WithTargetOutbound(n int) Opt {
	return func(c *Cluster) {
		c.targetOutbound = n
	}
}

// WithRerunInterval configures how often to rerun tortoise from scratch.
func WithRerunInterval(interval time.Duration) Opt {
	return func(c *Cluster) {
		c.rerunInterval = interval
	}
}

// WithKeys generates n prefunded keys.
func WithKeys(n int) Opt {
	return func(c *Cluster) {
		c.accounts = accounts{keys: genSigners(n)}
	}
}

// New initializes Cluster with options.
func New(opts ...Opt) *Cluster {
	cluster := &Cluster{
		image:          "spacemeshos/go-spacemesh-dev:develop",
		genesisTime:    time.Now().Add(time.Minute),
		targetOutbound: 3,
		rerunInterval:  60 * time.Minute,
	}
	for _, opt := range opts {
		opt(cluster)
	}
	return cluster
}

// Cluster for managing state of the spacemesh cluster.
type Cluster struct {
	image string

	genesisTime    time.Time
	targetOutbound int
	rerunInterval  time.Duration

	accounts

	bootnodes []*NodeClient
	smeshers  []*NodeClient
	clients   []*NodeClient
	poets     []string
}

// AddPoet ...
func (c *Cluster) AddPoet(cctx *clustercontext.Context) error {
	if len(c.bootnodes) == 0 {
		return fmt.Errorf("bootnodes are used as a gateway. please create a few before adding a poet server")
	}
	if len(c.poets) == 1 {
		return fmt.Errorf("currently only one poet is supported")
	}
	gateways := []string{}
	for _, bootnode := range c.bootnodes {
		gateways = append(gateways, fmt.Sprintf("dns:///%s.%s:9092", bootnode.Name, bootSvc))
	}
	endpoint, err := deployPoet(cctx, gateways...)
	if err != nil {
		return err
	}
	c.poets = append(c.poets, endpoint)
	return nil
}

// AddBootnodes ...
func (c *Cluster) AddBootnodes(cctx *clustercontext.Context, n int) error {
	smcfg := SMConfig{
		GenesisTime:    c.genesisTime,
		NetworkID:      defaultNetID,
		PoetEndpoint:   poetEndpoint(),
		Genesis:        genGenesis(c.keys),
		TargetOutbound: c.targetOutbound,
		RerunInterval:  c.rerunInterval,
	}
	dcfg := DeployConfig{
		Image:    c.image,
		Name:     "boot",
		Headless: bootSvc,
		Count:    int32(len(c.bootnodes) + n),
	}
	clients, err := deployNodes(cctx, dcfg, smcfg)
	if err != nil {
		return err
	}
	c.bootnodes = clients
	c.clients = nil
	c.clients = append(c.clients, c.bootnodes...)
	c.clients = append(c.clients, c.smeshers...)
	return nil
}

// AddSmeshers ...
func (c *Cluster) AddSmeshers(cctx *clustercontext.Context, n int) error {
	smcfg := SMConfig{
		Bootnodes:      extractP2PEndpoints(c.bootnodes),
		GenesisTime:    c.genesisTime,
		NetworkID:      defaultNetID,
		PoetEndpoint:   poetEndpoint(),
		Genesis:        genGenesis(c.keys),
		TargetOutbound: c.targetOutbound,
		RerunInterval:  c.rerunInterval,
	}
	dcfg := DeployConfig{
		Image:    c.image,
		Name:     "smesher",
		Headless: "smesher-headless",
		Count:    int32(len(c.smeshers) + n),
	}
	clients, err := deployNodes(cctx, dcfg, smcfg)
	if err != nil {
		return err
	}
	c.smeshers = clients
	c.clients = nil
	c.clients = append(c.clients, c.bootnodes...)
	c.clients = append(c.clients, c.smeshers...)
	return nil
}

// Total returns total number of clients.
func (c *Cluster) Total() int {
	return len(c.clients)
}

// Client returns client for i-th node, either bootnode or smesher.
func (c *Cluster) Client(i int) *NodeClient {
	return c.clients[i]
}

// Boot returns client for i-th bootnode.
func (c *Cluster) Boot(i int) *NodeClient {
	return c.bootnodes[i]
}

// Smesher returns client for i-th smesher.
func (c *Cluster) Smesher(i int) *NodeClient {
	return c.smeshers[i]
}

type accounts struct {
	keys []*signer
}

func (a *accounts) Private(i int) ed25519.PrivateKey {
	return a.keys[i].PK
}

func (a *accounts) Address(i int) string {
	return a.keys[i].Address()
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

func genSigners(n int) (rst []*signer) {
	for i := 0; i < n; i++ {
		rst = append(rst, genSigner())
	}
	return
}

func genSigner() *signer {
	pub, pk, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	return &signer{Pub: pub, PK: pk}
}

func extractNames(nodes []*NodeClient) []string {
	var rst []string
	for _, n := range nodes {
		rst = append(rst, n.Name)
	}
	return rst
}

func extractP2PEndpoints(nodes []*NodeClient) []string {
	var rst []string
	for _, n := range nodes {
		rst = append(rst, n.P2PEndpoint())
	}
	return rst
}
