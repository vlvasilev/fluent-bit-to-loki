package lokiplugin

import (
	"fmt"
	"regexp"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"go.etcd.io/etcd/client"

	controller "github.com/fluent-bit-to-loki/pkg/controller/controller"
	lokiclient "github.com/grafana/loki/pkg/promtail/client"
)

type Loki interface {
	SendRecord(r map[interface{}]interface{}, ts time.Time) error
	Close()
}

type loki struct {
	cfg               *config
	defaultClient     lokiclient.Client
	dynamicHostRegexp regexp.Regexp
	controller        *Controller
	logger            log.Logger
}

func NewPlugin(cfg *config, logger log.Logger) (*loki, error) {
	defaultLokiClient, err := lokiclient.New(cfg.clientConfig, logger)
	if err != nil {
		return nil, err
	}
	kubernetesCleint, err := getInclusterKubernetsClient()
	if err != nil {
		return nil, err
	}
	ctl, err := controller.NewController(kubernetesCleint, cfg.clientConfig, logger, cfg.dynamicHostRegex, cfg.dynamicHostPrefix, cfg.dynamicHostSulfix)
	if err != nil {
		return nil, err
	}
	return &loki{
		cfg:        cfg,
		client:     client,
		controller: ctl,
		logger:     logger,
	}, nil
}

// sendRecord send fluentbit records to loki as an entry.
func (l *loki) SendRecord(r map[interface{}]interface{}, ts time.Time) error {
	records := toStringMap(r)
	level.Debug(l.logger).Log("msg", "processing records", "records", fmt.Sprintf("%+v", records))
	lbs := model.LabelSet{}
	if l.cfg.autoKubernetesLabels {
		err := autoLabels(records, lbs)
		if err != nil {
			level.Error(l.logger).Log("msg", err.Error(), "records", fmt.Sprintf("%+v", records))
		}
	}
	if l.cfg.labelMap != nil {
		mapLabels(records, l.cfg.labelMap, lbs)
	} else {
		lbs = extractLabels(records, l.cfg.labelKeys)
	}

	dynamicHostName := getDynamicHostName(records, l.cfg.dynamicHostPath)
	removeKeys(records, append(l.cfg.labelKeys, l.cfg.removeKeys...))
	if len(records) == 0 {
		return nil
	}
	if l.cfg.dropSingleKey && len(records) == 1 {
		for _, v := range records {
			return l.client.Handle(lbs, ts, fmt.Sprintf("%v", v))
		}
	}
	line, err := createLine(records, l.cfg.lineFormat)
	if err != nil {
		return fmt.Errorf("error creating line: %v", err)
	}

	client := l.getClient(dynamicHostName)
	if client == nil {
		return fmt.Errorf("could not found client for %s", dynamicHostName)
	}

	return client.Handle(lbs, ts, line)
}

func (l *loki) Close() {
	loki.defaultClient.Stop()
	loki.controller.Stop()
}

func (l *loki) getClient(dynamicHosName string) client.Client {
	if dynamicHosName != "" && l.controller.isDynamicHost(dynamicHosName) {
		return l.controller.getClient(dynamicHosName)
	}

	return l.client
}
