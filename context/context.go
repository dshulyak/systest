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
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	imageFlag = flag.String("image", "spacemeshos/go-spacemesh-dev:fastnet",
		"go-spacemesh image")
	namespaceFlag = flag.String("namespace", "",
		"namespace for the cluster. if empty every test will use random namespace")
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

type Context struct {
	context.Context
	Client    *kubernetes.Clientset
	Generic   client.Client
	Namespace string
	Image     string
}

func cleanup(tb testing.TB, f func()) {
	tb.Cleanup(f)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	tb.Log("registered signal waiter")
	go func() {
		sig := <-signals
		tb.Logf("termination signal received. signal=%v", sig)
		f()
		os.Exit(1)
	}()
}

func deleteNamespace(ctx *Context) error {
	err := ctx.Client.CoreV1().Namespaces().Delete(ctx, ctx.Namespace, apimetav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", ctx.Namespace, err)
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

func New(ctx context.Context, tb testing.TB) (*Context, error) {
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
	cctx := &Context{
		Context:   ctx,
		Namespace: ns,
		Client:    clientset,
		Generic:   generic,
		Image:     *imageFlag,
	}
	cleanup(tb, func() {
		if err := deleteNamespace(cctx); err != nil {
			tb.Logf("cleanup failure. err=%v", err)
			return
		}
		tb.Log("cleanup complete")
	})
	if err := deployNamespace(cctx); err != nil {
		return nil, err
	}
	tb.Logf("using namespace. ns=%s", cctx.Namespace)
	return cctx, nil
}
