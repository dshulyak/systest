package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	clustercontext "github.com/dshulyak/systest/context"

	spacemeshv1 "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	metav1 "k8s.io/client-go/applyconfigurations/meta/v1"
)

type DeployConfig struct {
	Name     string
	Headless string
	Image    string
	Count    int32
}

type SMConfig struct {
	Bootnodes    []string
	GenesisTime  time.Time
	NetworkID    uint32
	PoetEndpoint string // "0.0.0.0:7777"
}

type Node struct {
	Name      string
	IP        string
	P2P, GRPC uint16
	ID        string
}

func (n Node) GRPCEndpoint() string {
	return fmt.Sprintf("%s:%d", n.IP, n.GRPC)
}

func (n Node) P2PEndpoint() string {
	return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", n.IP, n.P2P, n.ID)
}

type NodeClient struct {
	Node
	Conn *grpc.ClientConn
}

func DeployNamespace(ctx *clustercontext.Context) error {
	_, err := ctx.Client.CoreV1().Namespaces().Apply(ctx, corev1.Namespace(ctx.Namespace), apimetav1.ApplyOptions{FieldManager: "test"})
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", ctx.Namespace, err)
	}
	return nil
}

func Cleanup(ctx *clustercontext.Context) error {
	err := ctx.Client.CoreV1().Namespaces().Delete(ctx, ctx.Namespace, apimetav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", ctx.Namespace, err)
	}
	return nil
}

// DeployPoet accepts address of the gateway (to use dns resolver add dns:/// prefix to the address)
// and output ip of the poet
func DeployPoet(ctx *clustercontext.Context, gateways ...string) (string, error) {
	const port = 80
	args := []string{}
	for _, gateway := range gateways {
		args = append(args, "--gateway="+gateway)
	}
	args = append(args, "--restlisten=0.0.0.0:"+strconv.Itoa(port))
	args = append(args, "--n=19")
	pod := corev1.Pod("poet", ctx.Namespace).WithSpec(
		corev1.PodSpec().WithContainers(
			corev1.Container().
				WithName("poet").
				WithImage("spacemeshos/poet:ef8f28a").
				WithArgs(args...).
				WithPorts(corev1.ContainerPort().WithName("rest").WithProtocol("TCP").WithContainerPort(port)),
		),
	)
	_, err := ctx.Client.CoreV1().Pods(ctx.Namespace).Apply(ctx, pod, apimetav1.ApplyOptions{FieldManager: "test"})
	if err != nil {
		return "", fmt.Errorf("create poet: %w", err)
	}
	waited, err := waitPod(ctx, *pod.Name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", waited.Status.PodIP, port), nil
}

func waitPod(ctx *clustercontext.Context, name string) (*v1.Pod, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		pod, err := ctx.Client.CoreV1().Pods(ctx.Namespace).Get(ctx, name, apimetav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("read pod %s: %w", name, err)
		}
		switch pod.Status.Phase {
		case v1.PodFailed:
			return nil, fmt.Errorf("pod failed %s", name)
		case v1.PodRunning:
			return pod, nil
		}
	}
}

func DeployNodes(ctx *clustercontext.Context, bcfg DeployConfig, smcfg SMConfig) ([]*NodeClient, error) {
	labels := map[string]string{
		"app": bcfg.Name,
	}
	svc := corev1.Service(bcfg.Headless, ctx.Namespace).
		WithLabels(labels).
		WithSpec(corev1.ServiceSpec().WithSelector(labels).WithPorts(
			corev1.ServicePort().WithName("grpc").WithPort(9092).WithProtocol("TCP")))

	_, err := ctx.Client.CoreV1().Services(ctx.Namespace).Apply(ctx, svc, apimetav1.ApplyOptions{FieldManager: "test"})
	if err != nil {
		return nil, fmt.Errorf("apply headless service: %w", err)
	}

	sset := appsv1.StatefulSet(bcfg.Name, ctx.Namespace).
		WithSpec(appsv1.StatefulSetSpec().
			WithReplicas(bcfg.Count).
			WithServiceName(*svc.Name).
			WithVolumeClaimTemplates(
				corev1.PersistentVolumeClaim("data", ctx.Namespace).
					WithSpec(corev1.PersistentVolumeClaimSpec().
						WithAccessModes(v1.ReadWriteOnce).
						WithStorageClassName("standard").
						WithResources(corev1.ResourceRequirements().
							WithRequests(v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")}))),
			).
			WithSelector(metav1.LabelSelector().WithMatchLabels(labels)).
			WithTemplate(corev1.PodTemplateSpec().
				WithLabels(labels).
				WithSpec(corev1.PodSpec().WithContainers(corev1.Container().
					WithName("smesher").
					WithImage(bcfg.Image).
					WithPorts(
						corev1.ContainerPort().WithContainerPort(7513).WithName("p2p"),
						corev1.ContainerPort().WithContainerPort(9092).WithName("grpc"),
					).
					WithVolumeMounts(
						corev1.VolumeMount().WithName("data").WithMountPath("/data"),
					).
					WithCommand(
						"/bin/go-spacemesh",
						"--preset=testnet",
						"--smeshing-start=true",
						"--smeshing-opts-datadir=/data/post",
						"-d=/data/state",
						"--poet-server="+smcfg.PoetEndpoint,
						"--network-id="+strconv.Itoa(int(smcfg.NetworkID)),
						"--genesis-time="+smcfg.GenesisTime.Format(time.RFC3339),
						"--bootnodes="+strings.Join(smcfg.Bootnodes, ","),
						"--target-outbound=2",
						"--test-mode",
					),
				)),
			),
		)

	_, err = ctx.Client.AppsV1().StatefulSets(ctx.Namespace).Apply(ctx, sset, apimetav1.ApplyOptions{FieldManager: "test"})
	if err != nil {
		return nil, fmt.Errorf("apply statefulset: %w", err)
	}
	var result []*NodeClient
	for i := 0; i < int(bcfg.Count); i++ {
		attempt := func() error {
			name := fmt.Sprintf("%s-%d", *sset.Name, i)
			pod, err := waitPod(ctx, name)
			if err != nil {
				return err
			}
			node := Node{
				Name: name,
				IP:   pod.Status.PodIP,
				P2P:  7513,
				GRPC: 9092,
			}
			rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			conn, err := grpc.DialContext(rctx, node.GRPCEndpoint(), grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				return err
			}
			cancel()
			dbg := spacemeshv1.NewDebugServiceClient(conn)
			info, err := dbg.NetworkInfo(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			node.ID = info.Id
			result = append(result, &NodeClient{
				Node: node,
				Conn: conn,
			})
			return nil
		}
		const attempts = 10
		for i := 1; i <= attempts; i++ {
			if err := attempt(); err != nil && i == attempts {
				return nil, err
			} else if err == nil {
				break
			}
		}
	}
	return result, nil
}
