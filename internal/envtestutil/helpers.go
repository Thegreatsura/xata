package envtestutil

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultEventuallyTimeout  = 5 * time.Second
	DefaultEventuallyInterval = 100 * time.Millisecond
)

// RetryOnConflict tries to update the given object using the provided mutate
// function, retrying on conflict errors.
func RetryOnConflict[T client.Object](ctx context.Context, cl client.Client, obj T, mutateFn func(obj T)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := cl.Get(ctx, client.ObjectKey{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}, obj)
		if err != nil {
			return err
		}
		mutateFn(obj)
		return cl.Update(ctx, obj)
	})
}

// RetryStatusOnConflict tries to update the status of the given object using
// the provided mutate function, retrying on conflict errors.
func RetryStatusOnConflict[T client.Object](ctx context.Context, cl client.Client, obj T, mutateFn func(obj T)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := cl.Get(ctx, client.ObjectKey{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}, obj)
		if err != nil {
			return err
		}
		mutateFn(obj)
		return cl.Status().Update(ctx, obj)
	})
}

// RequireEventuallyTrue polls fn until it returns true, failing the test after
// DefaultEventuallyTimeout.
func RequireEventuallyTrue(t *testing.T, fn func() bool, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool {
		return fn()
	}, DefaultEventuallyTimeout, DefaultEventuallyInterval, msgAndArgs...)
}

// RequireEventuallyNoErr polls fn until it returns nil, failing the test after
// DefaultEventuallyTimeout.
func RequireEventuallyNoErr(t *testing.T, fn func() error, msgAndArgs ...any) {
	t.Helper()
	RequireEventuallyTrue(t, func() bool {
		return fn() == nil
	}, msgAndArgs...)
}

// RandomString generates a random lowercase string of the given length.
func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"

	b := make([]byte, n)
	for i := range b {
		//nolint:gosec
		b[i] = letters[rand.IntN(len(letters))]
	}

	return string(b)
}

// GetObject retrieves a Kubernetes object by name and namespace.
func GetObject(ctx context.Context, cl client.Client, name, namespace string, obj client.Object) error {
	return cl.Get(ctx, client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}, obj)
}
