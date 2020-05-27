package main

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	corev1 "k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	lokiclient "github.com/grafana/loki/pkg/promtail/client"
)

type Controller struct {
	namespaceLister   corev1lister.NamespaceLister
	lock              sync.RWMutex
	clients           map[string]lokiclient.Client
	clientConfig      lokiclient.Config
	dynamicHostPrefix string
	dynamicHostSulfix string
	dynamicHostRegex  *regexp.Regexp
	stopChn           chan struct{}
	logger            log.Logger
}

func NewController(clientConfig lokiclient.Config, logger log.Logger, dynamicHostRegex, dynamicHostPrefix, dynamicHostSulfix string) (*Controller, error) {
	if dynamicHostRegex == "" {
		return &Controller{}, nil
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error building kubernetes clientset: %s", err.Error())
	}
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)

	controller := &Controller{
		namespaceLister:   kubeInformerFactory.Core().V1().Namespaces().Lister(),
		clients:           make(map[string]lokiclient.Client),
		stopChn:           make(chan struct{}),
		clientConfig:      clientConfig,
		dynamicHostPrefix: dynamicHostPrefix,
		dynamicHostSulfix: dynamicHostSulfix,
		dynamicHostRegex:  regexp.MustCompile(dynamicHostRegex),
		logger:            logger,
	}

	kubeInformerFactory.Core().V1().Namespaces().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addFunc,
		DeleteFunc: controller.delFunc,
	})

	kubeInformerFactory.Start(controller.stopChn)
	if !cache.WaitForCacheSync(controller.stopChn, kubeInformerFactory.Core().V1().Namespaces().Informer().HasSynced) {
		return nil, fmt.Errorf("failed to wait for caches to sync")
	}

	return controller, nil
}

func (ctl *Controller) getClient(name string) lokiclient.Client {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()

	if ctl.clients == nil {
		return nil
	}

	if client, ok := ctl.clients[name]; ok {
		return client
	}
	return nil
}

func (ctl *Controller) addFunc(obj interface{}) {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()

	namespace := ctl.getNamespaceNameIfMatch(obj)
	if namespace == "" {
		return
	}
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

func (ctl *Controller) Stop() {
	ctl.lock.Lock()
	defer ctl.lock.Unlock()
	close(ctl.stopChn)
	for _, client := range ctl.clients {
		client.Stop()
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
