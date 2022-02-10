package chaos

import (
	"context"

	clustercontext "github.com/dshulyak/systest/context"

	chaosv1alpha1 "github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
)

// Teardown is returned by every chaos action and executed
// by the caller once chaos needs to be stopped.
type Teardown func(context.Context) error

// Fail the list of pods and prevents them from respawning until teardown is called.
func Fail(cctx *clustercontext.Context, name string, pods ...string) (error, Teardown) {
	fail := chaosv1alpha1.PodChaos{}
	fail.Name = name
	fail.Namespace = cctx.Namespace

	fail.Spec.Action = chaosv1alpha1.PodFailureAction
	fail.Spec.Mode = chaosv1alpha1.AllMode
	fail.Spec.Selector = chaosv1alpha1.PodSelectorSpec{
		Pods: map[string][]string{
			cctx.Namespace: pods,
		},
	}
	if err := cctx.Generic.Create(cctx, &fail); err != nil {
		return err, nil
	}
	return nil, func(ctx context.Context) error {
		return cctx.Generic.Delete(ctx, &fail)
	}
}
