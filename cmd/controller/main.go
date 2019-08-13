package main

import (
	"flag"
	"time"

	"github.com/caicloud/crd-sample/pkg/client/clientset"
	informers "github.com/caicloud/crd-sample/pkg/client/informers"
	"github.com/caicloud/crd-sample/pkg/controller/labels"
	"github.com/golang/glog"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	masterURL  string
	kubeconfig string
)

func main() {
	flag.Parse()

	stopCh := make(chan struct{})

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	labelsClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	labelsInformerFactory := informers.NewSharedInformerFactory(labelsClient, time.Second*30)

	controller := labels.NewLabelCounterController(&labels.LabelCounterControllerOptions{
		KubeClient:           kubeClient,
		LabelsClient:         labelsClient,
		LabelCounterInformer: labelsInformerFactory.Labels().V1alpha1().LabelCounters(),
		NodeInformer:         kubeInformerFactory.Core().V1().Nodes(),
	})

	go kubeInformerFactory.Start(stopCh)
	go labelsInformerFactory.Start(stopCh)

	controller.Run(2, stopCh)
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}
