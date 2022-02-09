package context

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	chaosoperatorv1alpha1 "github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	imageFlag = flag.String("image", "spacemeshos/go-spacemesh-dev:proposal-events",
		"go-spacemesh image")
	namespaceFlag = flag.String("namespace", "",
		"namespace for the cluster. if empty every test will use random namespace")
	logLevel          = zap.LevelFlag("level", zap.InfoLevel, "verbosity of the logger")
	bootstrapDuration = flag.Duration("bootstrap", 30*time.Second,
		"bootstrap time is added to the genesis time. it may take longer on cloud environmens due to the additional resource management")
	clusterSize  = flag.Int("size", 10, "size of the cluster")
	testTimeout  = flag.Duration("test-timeout", 30*time.Minute, "timeout for a single test")
	nodeSelector = map[string]string{}
)

func init() {
	flag.Var(stringToString(nodeSelector), "node-selector", "select where test pods will be scheduled")
}

func rngName() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	const choices = "qwertyuiopasdfghjklzxcvbnm"
	buf := make([]byte, 4)
	for i := range buf {
		buf[i] = choices[rng.Intn(len(choices))]
	}
	return string(buf)
}

type Context struct {
	context.Context
	Client            *kubernetes.Clientset
	BootstrapDuration time.Duration
	ClusterSize       int
	Generic           client.Client
	Namespace         string
	Image             string
	NodeSelector      map[string]string
	Log               *zap.SugaredLogger
}

func cleanup(tb testing.TB, f func()) {
	tb.Cleanup(f)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		f()
		os.Exit(1)
	}()
}

func deleteNamespace(ctx *Context) error {
	err := ctx.Client.CoreV1().Namespaces().Delete(ctx, ctx.Namespace, apimetav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete namespace %s: %w", ctx.Namespace, err)
	}
	return nil
}

func deployNamespace(ctx *Context) error {
	_, err := ctx.Client.CoreV1().Namespaces().Apply(ctx, corev1.Namespace(ctx.Namespace),
		apimetav1.ApplyOptions{FieldManager: "test"})
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", ctx.Namespace, err)
	}
	return nil
}

func New(tb testing.TB) (*Context, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	ns := *namespaceFlag
	if len(ns) == 0 {
		ns = "test-" + rngName()
	}
	scheme := runtime.NewScheme()
	if err := chaosoperatorv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	generic, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *testTimeout)
	tb.Cleanup(cancel)
	cctx := &Context{
		Context:           ctx,
		Namespace:         ns,
		BootstrapDuration: *bootstrapDuration,
		Client:            clientset,
		Generic:           generic,
		ClusterSize:       *clusterSize,
		Image:             *imageFlag,
		NodeSelector:      nodeSelector,
		Log:               zaptest.NewLogger(tb, zaptest.Level(logLevel)).Sugar(),
	}
	cleanup(tb, func() {
		if err := deleteNamespace(cctx); err != nil {
			cctx.Log.Errorf("cleanup failed", "error", err)
			return
		}
		cctx.Log.Debug("cleanup completed")
	})
	if err := deployNamespace(cctx); err != nil {
		return nil, err
	}
	cctx.Log.Infow("using", "namespace", cctx.Namespace)
	return cctx, nil
}
