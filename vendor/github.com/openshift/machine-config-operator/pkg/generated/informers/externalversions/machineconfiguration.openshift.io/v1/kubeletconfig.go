// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	"context"
	time "time"

	machineconfigurationopenshiftiov1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	versioned "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/openshift/machine-config-operator/pkg/generated/informers/externalversions/internalinterfaces"
	v1 "github.com/openshift/machine-config-operator/pkg/generated/listers/machineconfiguration.openshift.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// KubeletConfigInformer provides access to a shared informer and lister for
// KubeletConfigs.
type KubeletConfigInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.KubeletConfigLister
}

type kubeletConfigInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewKubeletConfigInformer constructs a new informer for KubeletConfig type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewKubeletConfigInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredKubeletConfigInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredKubeletConfigInformer constructs a new informer for KubeletConfig type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredKubeletConfigInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.MachineconfigurationV1().KubeletConfigs().List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.MachineconfigurationV1().KubeletConfigs().Watch(context.TODO(), options)
			},
		},
		&machineconfigurationopenshiftiov1.KubeletConfig{},
		resyncPeriod,
		indexers,
	)
}

func (f *kubeletConfigInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredKubeletConfigInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *kubeletConfigInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&machineconfigurationopenshiftiov1.KubeletConfig{}, f.defaultInformer)
}

func (f *kubeletConfigInformer) Lister() v1.KubeletConfigLister {
	return v1.NewKubeletConfigLister(f.Informer().GetIndexer())
}