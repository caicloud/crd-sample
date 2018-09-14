package labels

import (
	"fmt"
	"testing"
	"time"

	"github.com/caicloud/crd-sample/pkg/apis/labels/v1alpha1"
	"github.com/caicloud/crd-sample/pkg/client/clientset/fake"
	informers "github.com/caicloud/crd-sample/pkg/client/informers"
	labelsinformers "github.com/caicloud/crd-sample/pkg/client/informers/labels/v1alpha1"
	utiltesting "github.com/caicloud/crd-sample/pkg/utils/testing"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeinformers "k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

var (
	noResyncPeriodFunc = func() time.Duration { return 0 }

	labelcountersResource = schema.GroupVersionResource{Group: "labels.caicloud.io", Version: "v1alpha1", Resource: "labelcounters"}
)

type fixture struct {
	t *testing.T

	labelsClient *fake.Clientset
	kubeClient   *kubefake.Clientset

	labelCounterInformer labelsinformers.LabelCounterInformer
	nodeInformer         coreinformers.NodeInformer

	// Objects to put in the store.
	labelCounterStore []runtime.Object
	nodeStore         []runtime.Object

	// Actions expected to happen
	expectedlabelsAction []kubetesting.Action
	expectedkubeAction   []kubetesting.Action

	ignoredAction []utiltesting.VerbAndResource

	synced chan struct{}
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.ignoredAction = []utiltesting.VerbAndResource{
		{
			Verb:     "list",
			Resource: "labelcounters",
		},
		{
			Verb:     "watch",
			Resource: "labelcounters",
		},
	}
	return f
}

func newLabelCounter(key string, values []string, valueCounts map[string]int) *v1alpha1.LabelCounter {
	lc := &v1alpha1.LabelCounter{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: key,
		},
		Spec: v1alpha1.LabelCounterSpec{
			Values: values,
		},
	}
	for _, value := range values {
		count, ok := valueCounts[value]
		if ok {
			lc.Status.Counters = append(lc.Status.Counters, v1alpha1.Counter{
				Value: value,
				Count: count,
			})
		}
	}
	return lc
}

func newNode(name string, labels map[string]string) *v1.Node {
	return &v1.Node{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func (f *fixture) fakeLabelCounterController(stopCh chan struct{}) *LabelCounterController {
	f.labelsClient = fake.NewSimpleClientset(f.labelCounterStore...)
	f.kubeClient = kubefake.NewSimpleClientset(f.nodeStore...)

	fakeLabelsInformers := informers.NewSharedInformerFactory(f.labelsClient, noResyncPeriodFunc())
	fakeKubeInformers := kubeinformers.NewSharedInformerFactory(f.kubeClient, noResyncPeriodFunc())

	fakeLabelCounterInformer := fakeLabelsInformers.Labels().V1alpha1().LabelCounters()
	fakeNodeInformer := fakeKubeInformers.Core().V1().Nodes()

	for _, lc := range f.labelCounterStore {
		fakeLabelCounterInformer.Informer().GetIndexer().Add(lc)
	}

	for _, n := range f.nodeStore {
		fakeNodeInformer.Informer().GetIndexer().Add(n)
	}

	fakeController := NewLabelCounterController(&LabelCounterControllerOptions{
		KubeClient:           f.kubeClient,
		LabelsClient:         f.labelsClient,
		LabelCounterInformer: fakeLabelCounterInformer,
		NodeInformer:         fakeNodeInformer,
	})

	f.labelCounterInformer = fakeLabelCounterInformer
	f.nodeInformer = fakeNodeInformer

	go fakeLabelsInformers.Start(stopCh)
	go fakeKubeInformers.Start(stopCh)

	return fakeController
}

type EventTrigger func(f *fixture) error

func (f *fixture) run(et EventTrigger, eventCount int) error {
	stopCh := make(chan struct{})
	defer close(stopCh)

	c := f.fakeLabelCounterController(stopCh)
	// Wait for cache syncing so that controller actions will be always
	// after trigger actions
	if !cache.WaitForCacheSync(stopCh, c.informersSynced...) {
		return fmt.Errorf("can't sync cache for label counter controller")
	}

	// clear queue after informer sync
	for i := 0; i < c.queue.Len(); i++ {
		item, _ := c.queue.Get()
		c.queue.Done(item)
	}

	// clean adding and syncing actions when controller runs
	f.labelsClient.ClearActions()
	f.kubeClient.ClearActions()

	oldSyncHandler := c.syncHandler
	defer func() {
		c.syncHandler = oldSyncHandler
	}()
	c.syncHandler = func(key string) error {
		err := oldSyncHandler(key)
		if err != nil {
			return err
		}
		f.synced <- struct{}{}
		return nil
	}

	f.synced = make(chan struct{})
	defer close(f.synced)

	if err := et(f); err != nil {
		return err
	}

	go c.Run(1, stopCh)

	for i := 0; i < eventCount; i++ {
		<-f.synced
	}

	return nil
}

func createLabelCounters(lcs ...*v1alpha1.LabelCounter) EventTrigger {
	return func(f *fixture) error {
		for _, lc := range lcs {
			if _, err := f.labelsClient.LabelsV1alpha1().LabelCounters().Create(lc); err != nil {
				return err
			}
		}
		return nil
	}
}

func testCreateLabalCounter(f *fixture, creations, updations []*v1alpha1.LabelCounter) {
	f.run(createLabelCounters(creations...), len(creations))

	actions := f.labelsClient.Actions()

	var expected []kubetesting.Action

	for _, u := range updations {
		expected = append(expected, kubetesting.NewRootUpdateAction(labelcountersResource, u))
	}

	utiltesting.AssertActionCounts(f.t, map[string]int{
		"get":    0,
		"update": len(updations),
		"create": len(creations),
		"list":   0,
		"watch":  0,
	}, actions)

	utiltesting.AssertActions(f.t, expected, actions[len(creations):], f.ignoredAction)
}

func TestCreateLabelCounter(t *testing.T) {
	normalNodes := []runtime.Object{
		newNode("node-1", nil),
		newNode("node-2", map[string]string{
			"aaa": "bbb",
		}),
		newNode("node-3", map[string]string{
			"labels.caicloud.io/test": "nnn",
		}),
	}
	cases := []struct {
		description string
		nodes       []runtime.Object
		creations   []*v1alpha1.LabelCounter
		updations   []*v1alpha1.LabelCounter
	}{
		{
			description: "labels.caicloud.io/test:nnn can be count and update label counter status",
			nodes:       normalNodes,
			creations: []*v1alpha1.LabelCounter{
				newLabelCounter("test", []string{"nnn"}, nil),
			},
			updations: []*v1alpha1.LabelCounter{
				newLabelCounter("test", []string{"nnn"}, map[string]int{"nnn": 1}),
			},
		},
		{
			description: "aaa:bbb status will be updated to 0 because of labels prefix",
			nodes:       normalNodes,
			creations: []*v1alpha1.LabelCounter{
				newLabelCounter("aaa", []string{"bbb"}, nil),
			},
			updations: []*v1alpha1.LabelCounter{
				newLabelCounter("aaa", []string{"bbb"}, map[string]int{"bbb": 0}),
			},
		},
	}

	f := newFixture(t)

	for _, c := range cases {
		f.nodeStore = c.nodes
		testCreateLabalCounter(f, c.creations, c.updations)
	}
}

func updateLabelCounters(lcs ...*v1alpha1.LabelCounter) EventTrigger {
	return func(f *fixture) error {
		for _, lc := range lcs {
			if _, err := f.labelsClient.LabelsV1alpha1().LabelCounters().Update(lc); err != nil {
				return err
			}
		}
		return nil
	}
}

func testUpdateLabalCounter(f *fixture, old *v1alpha1.LabelCounter, news []*v1alpha1.LabelCounter, updated *v1alpha1.LabelCounter) {
	f.labelCounterStore = append(f.labelCounterStore, old)
	// update 'new' will trigger syncing
	f.run(updateLabelCounters(news...), 1)

	actions := f.labelsClient.Actions()

	var expected []kubetesting.Action

	expected = append(expected, kubetesting.NewRootUpdateAction(labelcountersResource, updated))

	utiltesting.AssertActionCounts(f.t, map[string]int{
		"get":    0,
		"update": len(news) + 1,
		"create": 0,
		"list":   0,
		"watch":  0,
	}, actions)

	utiltesting.AssertActions(f.t, expected, actions[len(news):], f.ignoredAction)
}

func TestUpdateLabelCounter(t *testing.T) {
	normalNodes := []runtime.Object{
		newNode("node-1", map[string]string{
			"labels.caicloud.io/test": "aaa",
		}),
		newNode("node-2", map[string]string{
			"labels.caicloud.io/test": "bbb",
		}),
		newNode("node-3", map[string]string{
			"labels.caicloud.io/test": "ccc",
		}),
	}
	cases := []struct {
		description string
		nodes       []runtime.Object
		old         *v1alpha1.LabelCounter
		news        []*v1alpha1.LabelCounter
		updated     *v1alpha1.LabelCounter
	}{
		{
			description: "label counter test can be used to count another value",
			nodes:       normalNodes,
			old:         newLabelCounter("test", []string{"aaa"}, map[string]int{"aaa": 1}),
			news: []*v1alpha1.LabelCounter{
				newLabelCounter("test", []string{"aaa", "bbb"}, nil),
				newLabelCounter("test", []string{"aaa", "bbb", "ccc"}, nil),
			},
			updated: newLabelCounter("test", []string{"aaa", "bbb", "ccc"}, map[string]int{"aaa": 1, "bbb": 1, "ccc": 1}),
		},
	}

	f := newFixture(t)

	for _, c := range cases {
		f.nodeStore = c.nodes
		testUpdateLabalCounter(f, c.old, c.news, c.updated)
	}
}

func TestCreateNode(t *testing.T) {

}

func TestUpdateNode(t *testing.T) {

}

func TestDeleteNode(t *testing.T) {

}

// TODO(liubog2008): need to add a way to create missing events
func TestDeleteFinalStateUnknownNode(t *testing.T) {
}
