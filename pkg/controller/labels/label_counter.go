package labels

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/caicloud/crd-sample/pkg/apis/labels/v1alpha1"
	"github.com/caicloud/crd-sample/pkg/client/clientset"
	informers "github.com/caicloud/crd-sample/pkg/client/informers/labels/v1alpha1"
	listers "github.com/caicloud/crd-sample/pkg/client/listers/labels/v1alpha1"
	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type LabelCounterControllerOptions struct {
	KubeClient kubernetes.Interface

	LabelsClient clientset.Interface

	LabelCounterInformer informers.LabelCounterInformer

	NodeInformer coreinformers.NodeInformer
}

// LabelCounterController will count number of nodes which have specfied labels
type LabelCounterController struct {
	// kubeclient is a standard clientset
	kubeclient kubernetes.Interface
	// labelsclient is custom clientset
	labelsclient clientset.Interface

	labelCounterLister listers.LabelCounterLister
	nodeLister         corelisters.NodeLister

	informersSynced []cache.InformerSynced

	// queue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	queue workqueue.RateLimitingInterface

	syncHandler func(key string) error
}

func NewLabelCounterController(options *LabelCounterControllerOptions) *LabelCounterController {
	lc := LabelCounterController{
		kubeclient:         options.KubeClient,
		labelsclient:       options.LabelsClient,
		labelCounterLister: options.LabelCounterInformer.Lister(),
		nodeLister:         options.NodeInformer.Lister(),
		informersSynced: []cache.InformerSynced{
			options.LabelCounterInformer.Informer().HasSynced,
			options.NodeInformer.Informer().HasSynced,
		},
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "label counter"),
	}

	options.LabelCounterInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lc.addLabelCounter,
			UpdateFunc: lc.updateLabelCounter,
		},
	)

	options.NodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lc.addNode,
			UpdateFunc: lc.updateNode,
			DeleteFunc: lc.deleteNode,
		},
	)
	lc.syncHandler = lc.syncLabelCounterKey
	return &lc
}

// addLabelCounter adds obj watched into the working queue to serialize the operation
func (lc *LabelCounterController) addLabelCounter(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		glog.Errorf("couldn't get key for object %+v: %v", obj, err)
		return
	}
	lc.queue.Add(key)
}

func (lc *LabelCounterController) updateLabelCounter(old, cur interface{}) {
	oldLabelCounter := old.(*v1alpha1.LabelCounter)
	curLabelCounter := cur.(*v1alpha1.LabelCounter)

	if !reflect.DeepEqual(&oldLabelCounter.Spec, &curLabelCounter.Spec) {
		lc.addLabelCounter(cur)
	}
}

// addNode adds watched node into the working queue to serialize the operation
func (lc *LabelCounterController) addNode(obj interface{}) {
	node := obj.(*v1.Node)

	lc.enqueueLabelCounters(node.Labels)
}

func (lc *LabelCounterController) updateNode(old, cur interface{}) {
	oldNode := old.(*v1.Node)
	curNode := cur.(*v1.Node)

	updated := updatedLabels(oldNode.Labels, curNode.Labels)

	lc.enqueueLabelCounters(updated)
}

func (lc *LabelCounterController) deleteNode(obj interface{}) {
	node, ok := obj.(*v1.Node)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		node, ok = tombstone.Obj.(*v1.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a node %#v", obj))
			return
		}
	}
	lc.enqueueLabelCounters(node.Labels)
}

func (lc *LabelCounterController) enqueueLabelCounters(nodeLabels map[string]string) {
	labelCounters, err := lc.resolveNodeLabel(nodeLabels)
	if err != nil {
		utilruntime.HandleError(err)
	}
	for _, labelCounter := range labelCounters {
		lc.addLabelCounter(labelCounter)
	}
}

func updatedLabels(old, cur map[string]string) map[string]string {
	updated := map[string]string{}
	for k, v := range old {
		if !strings.HasPrefix(k, v1alpha1.LabelPrefix) {
			continue
		}
		curV, ok := cur[k]
		if !ok {
			continue
		}
		if curV != v {
			updated[k] = curV
		}
	}
	for k, v := range cur {
		if !strings.HasPrefix(k, v1alpha1.LabelPrefix) {
			continue
		}
		_, ok := updated[k]
		if !ok {
			updated[k] = v
		}
	}
	return updated
}

func (lc *LabelCounterController) resolveNodeLabel(nodeLabels map[string]string) ([]*v1alpha1.LabelCounter, error) {
	var labelCounters []*v1alpha1.LabelCounter
	for k := range nodeLabels {
		if strings.HasPrefix(k, v1alpha1.LabelPrefix) {
			key := strings.TrimPrefix(k, v1alpha1.LabelPrefix)
			labelCounter, err := lc.labelCounterLister.Get(key)
			if err != nil {
				if !errors.IsNotFound(err) {
					return nil, err
				}
				continue
			}
			labelCounters = append(labelCounters, labelCounter)
		}
	}
	return labelCounters, nil
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the resource specified by label spec
// with the current status of the label.
func (lc *LabelCounterController) syncLabelCounterKey(key string) error {
	labelCounter, err := lc.labelCounterLister.Get(key)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if err := lc.syncLabelCounter(labelCounter); err != nil {
		return err
	}
	return nil
}

func (lc *LabelCounterController) syncLabelCounter(labelCounter *v1alpha1.LabelCounter) error {
	counters := []v1alpha1.Counter{}
	for _, v := range labelCounter.Spec.Values {
		selector := labels.SelectorFromSet(labels.Set{
			v1alpha1.LabelPrefix + labelCounter.Name: v,
		})

		nodes, err := lc.nodeLister.List(selector)
		if err != nil {
			return err
		}
		counters = append(counters, v1alpha1.Counter{
			Value: v,
			Count: len(nodes),
		})
	}
	copy := labelCounter.DeepCopy()
	copy.Status.Counters = counters
	if _, err := lc.labelsclient.LabelsV1alpha1().LabelCounters().Update(copy); err != nil {
		return err
	}
	return nil
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
func (lc *LabelCounterController) worker(queue workqueue.RateLimitingInterface) func() {
	workFunc := func() bool {
		key, quit := queue.Get()
		if quit {
			return true
		}
		defer queue.Done(key)

		if err := lc.syncHandler(key.(string)); err != nil {
			utilruntime.HandleError(err)
			queue.AddRateLimited(key)
			return false
		}
		queue.Forget(key)
		return false
	}

	return func() {
		for {
			if quit := workFunc(); quit {
				glog.Infof("label controller worker shutting down")
				return
			}
		}
	}
}

// Run will start the label controller
func (lc *LabelCounterController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer lc.queue.ShutDown()

	glog.Infof("Starting label controller")
	defer glog.Infof("Shutting down label controller")

	if !cache.WaitForCacheSync(stopCh, lc.informersSynced...) {
		utilruntime.HandleError(fmt.Errorf("Unable to sync caches for label counter controller"))
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(lc.worker(lc.queue), time.Second, stopCh)
	}
	<-stopCh
}
