package ingress

import (
	"context"
	"kourier/pkg/envoy"

	"knative.dev/pkg/tracker"

	"knative.dev/serving/pkg/apis/networking/v1alpha1"

	"knative.dev/pkg/network"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubeclient "k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	nv1alpha1lister "knative.dev/serving/pkg/client/listers/networking/v1alpha1"
)

type Reconciler struct {
	IngressLister   nv1alpha1lister.IngressLister
	EndpointsLister corev1listers.EndpointsLister
	EnvoyXDSServer  envoy.EnvoyXdsServer
	kubeClient      kubeclient.Interface
	CurrentCaches   *envoy.Caches
	tracker         tracker.Interface
	statusManager   StatusProber
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	ingress, err := reconciler.IngressLister.Ingresses(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		reconciler.deleteIngress(namespace, name)
		return nil
	} else if err != nil {
		return err
	}

	reconciler.updateIngress(ingress)
	return nil
}

func (reconciler *Reconciler) deleteIngress(namespace, name string) {

	ingress := reconciler.CurrentCaches.GetIngress(name, namespace)

	// We need to check for ingress not being nil, because we can receive an event from an already
	// removed ingress, like for example, when the endpoints object for that ingress is updated/removed.
	if ingress != nil {
		reconciler.statusManager.CancelIngress(ingress)
	}
	reconciler.CurrentCaches.DeleteIngressInfo(name, namespace, reconciler.kubeClient)
	reconciler.EnvoyXDSServer.SetSnapshotForCaches(reconciler.CurrentCaches, nodeID)
}

func (reconciler *Reconciler) updateIngress(ingress *v1alpha1.Ingress) {

	envoy.UpdateInfoForIngress(
		reconciler.CurrentCaches,
		ingress,
		reconciler.kubeClient,
		reconciler.EndpointsLister,
		network.GetClusterDomainName(),
		reconciler.tracker,
	)

	reconciler.EnvoyXDSServer.SetSnapshotForCaches(reconciler.CurrentCaches, nodeID)

	_, _ = reconciler.statusManager.IsReady(ingress)
}
