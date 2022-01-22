package chaos

import (
	"context"

	chaosv1alpha1 "github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Context struct {
	context.Context
	Client    client.Client
	Namespace string
}

// Partition2 partitions pods in array a from pods in array b.
func Partition2(ctx *Context, a, b []string) (error, func(context.Context) error) {
	partition := chaosv1alpha1.NetworkChaos{}
	partition.Name = "rename-me"
	partition.Namespace = ctx.Namespace

	partition.Spec.Action = chaosv1alpha1.PartitionAction
	partition.Spec.Mode = chaosv1alpha1.AllMode
	partition.Spec.Selector.Pods = map[string][]string{
		ctx.Namespace: a,
	}
	partition.Spec.Direction = chaosv1alpha1.Both
	partition.Spec.Target = &chaosv1alpha1.PodSelector{
		Mode: chaosv1alpha1.AllMode,
	}
	partition.Spec.Target.Selector.Pods = map[string][]string{
		ctx.Namespace: b,
	}

	desired := partition.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, ctx.Client, &partition, func() error {
		partition.Spec = desired.Spec
		return nil
	})

	return err, func(rctx context.Context) error {
		return ctx.Client.Delete(rctx, &partition)
	}
}
