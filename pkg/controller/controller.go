package controller

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	corev1 "k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	lokiclient "github.com/grafana/loki/pkg/promtail/client"
)

type Controller interface {
	GetCleint(name string) lokiclient.Client
	Stop()
}

type controller struct {
	lock              sync.RWMutex
	clients           map[string]lokiclient.Client
	clientConfig      lokiclient.Config
	labelSelector     labels.Selector
	dynamicHostPrefix string
	dynamicHostSulfix string
	stopChn           chan struct{}
	logger            log.Logger
}

func NewController(client kubernetes.Interface, clientConfig lokiclient.Config, logger log.Logger, dynamicHostPrefix, dynamicHostSulfix string, l map[string]string) (Controller, error) {
	// if dynamicHostRegex == "" {
	// 	return &Controller{}, nil
	// }
	// config, err := rest.InClusterConfig()
	// if err != nil {
	// 	return nil, err
	// }
	// fake.NewSimpleClientset().
	// kubeClient, err := kubernetes.NewForConfig(config)
	// if err != nil {
	// 	return nil, fmt.Errorf("Error building kubernetes clientset: %s", err.Error())
	// }
	labelSelector := labels.SelectorFromSet(l)

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(client, time.Second*30)

	controller := &Controller{
		clients:           make(map[string]lokiclient.Client),
		stopChn:           make(chan struct{}),
		clientConfig:      clientConfig,
		labelSelector:     labelSelector,
		dynamicHostPrefix: dynamicHostPrefix,
		dynamicHostSulfix: dynamicHostSulfix,
		dynamicHostRegex:  regexp.MustCompile(dynamicHostRegex),
		logger:            logger,
	}

	kubeInformerFactory.Core().V1().Namespaces().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addFunc,
		DeleteFunc: controller.delFunc,
		UpdateFunc: controller.updateFunc,
	})

	kubeInformerFactory.Start(controller.stopChn)
	if !cache.WaitForCacheSync(controller.stopChn, kubeInformerFactory.Core().V1().Namespaces().Informer().HasSynced) {
		return nil, fmt.Errorf("failed to wait for caches to sync")
	}

	return controller, nil
}

func (ctl *Controller) GetClient(name string) lokiclient.Client {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()

	if client, ok := ctl.clients[name]; ok {
		return client
	}
	return nil
}

func (ctl *Controller) Stop() {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()
	close(ctl.stopChn)
	for _, client := range ctl.clients {
		client.Stop()
	}
}

func (ctl *Controller) addFunc(obj interface{}) {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()

	selector := labels.SelectorFromSet(labels.Set{"gardener.cloud/role": "shoot"})
	namespace, ok := obj.(*corev1.Namespace)
	if !ok {
		level.Error(ctl.logger).Log(fmt.Sprintf("%v", obj), "is not a namespace")
		return
	}

	if selector.Matches(namespace.Labels) {
		clientConf := ctl.getClientConfig(namespace)
		if clientConf == nil {
			return
		}

		client, err := lokiclient.New(*clientConf, logger)
		if err != nil {
			level.Error(ctl.logger).Log("failed to make new loki client for namespace", namespace, "error", err.Error())
			return
		}

		level.Info(ctl.logger).Log("Add client for namespace ", namespace)
		ctl.clients[namespace] = client
	}
}

func (ctl *Controller) delFunc(obj interface{}) {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()

	namespace := ctl.getNamespaceNameIfMatch(obj)
	if namespace == "" {
		return
	}

	client, ok := ctl.clients[namespace]
	if ok && client != nil {
		client.Stop()
		delete(ctl.clients, namespace)
	}
}

func (ctl *Controller) getNamespaceNameIfMatch(obj interface{}) string {
	namespace, ok := obj.(*corev1.Namespace)
	if !ok {
		level.Error(ctl.logger).Log(fmt.Sprintf("%v", obj), "is not a namespace")
		return ""
	}
	if !ctl.isDynamicHost(namespace.Name) {
		return ""
	}
	return namespace.Name
}

func (ctl *Controller) getClientConfig(namespaces string) *lokiclient.Config {
	var clientURL flagext.URLValue

	url := ctl.dynamicHostPrefix + namespaces + ctl.dynamicHostSulfix
	err := clientURL.Set(url)
	if err != nil {
		level.Error(ctl.logger).Log("failed to parse client URL", namespaces, "error", err.Error())
		return nil
	}

	clientConf := ctl.clientConfig
	clientConf.URL = clientURL

	return &clientConf
}

func (ctl *Controller) isDynamicHost(dynamicHostName string) bool {
	return ctl.dynamicHostRegex.Match([]byte(dynamicHostName))
}
