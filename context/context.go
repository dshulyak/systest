package context

import (
	"context"
	"math/rand"
	"time"

	chaosoperatorv1alpha1 "github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

func New(ctx context.Context) (*Context, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	ns := "test-" + rngName()
	scheme := runtime.NewScheme()
	if err := chaosoperatorv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	generic, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return &Context{
		Context:   ctx,
		Namespace: ns,
		Client:    clientset,
		Generic:   generic,
	}, nil
}
