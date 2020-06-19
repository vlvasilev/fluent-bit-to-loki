package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/prometheus/common/model"
	"github.com/weaveworks/common/logging"

	"github.com/grafana/loki/pkg/logql"
	"github.com/grafana/loki/pkg/promtail/client"
	lokiflag "github.com/grafana/loki/pkg/util/flagext"
)

var defaultClientCfg = client.Config{}

func init() {
	// Init everything with default values.
	flagext.RegisterFlags(&defaultClientCfg)
}

type ConfigGetter interface {
	Get(key string) string
}

type Format int

const (
	JsonFormat Format = iota
	KvPairFormat
)

type Config struct {
	ClientConfig         client.Config
	LogLevel             logging.Level
	AutoKubernetesLabels bool
	RemoveKeys           []string
	LabelKeys            []string
	LineFormat           Format
	DropSingleKey        bool
	LabelMap             map[string]interface{}
	LabelSelector        map[string]string
	DynamicHostPath      map[string]interface{}
	DynamicHostPrefix    string
	DynamicHostSulfix    string
	DynamicHostRegex     string
}

func ParseConfig(cfg ConfigGetter) (*Config, error) {
	res := &Config{}

	res.ClientConfig = defaultClientCfg

	url := cfg.Get("URL")
	var clientURL flagext.URLValue
	if url == "" {
		url = "http://localhost:3100/loki/api/v1/push"
	}
	err := clientURL.Set(url)
	if err != nil {
		return nil, errors.New("failed to parse client URL")
	}
	res.ClientConfig.URL = clientURL

	// cfg.Get will return empty string if not set, which is handled by the client library as no tenant
	res.ClientConfig.TenantID = cfg.Get("TenantID")

	batchWait := cfg.Get("BatchWait")
	if batchWait != "" {
		batchWaitValue, err := strconv.Atoi(batchWait)
		if err != nil {
			return nil, fmt.Errorf("failed to parse BatchWait: %s", batchWait)
		}
		res.ClientConfig.BatchWait = time.Duration(batchWaitValue) * time.Second
	}

	batchSize := cfg.Get("BatchSize")
	if batchSize != "" {
		batchSizeValue, err := strconv.Atoi(batchSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse BatchSize: %s", batchSize)
		}
		res.ClientConfig.BatchSize = batchSizeValue
	}

	labels := cfg.Get("Labels")
	if labels == "" {
		labels = `{job="fluent-bit"}`
	}
	matchers, err := logql.ParseMatchers(labels)
	if err != nil {
		return nil, err
	}
	labelSet := make(model.LabelSet)
	for _, m := range matchers {
		labelSet[model.LabelName(m.Name)] = model.LabelValue(m.Value)
	}
	res.ClientConfig.ExternalLabels = lokiflag.LabelSet{LabelSet: labelSet}

	logLevel := cfg.Get("LogLevel")
	if logLevel == "" {
		logLevel = "info"
	}
	var level logging.Level
	if err := level.Set(logLevel); err != nil {
		return nil, fmt.Errorf("invalid log level: %v", logLevel)
	}
	res.LogLevel = level

	autoKubernetesLabels := cfg.Get("AutoKubernetesLabels")
	switch autoKubernetesLabels {
	case "false", "":
		res.AutoKubernetesLabels = false
	case "true":
		res.AutoKubernetesLabels = true
	default:
		return nil, fmt.Errorf("invalid boolean AutoKubernetesLabels: %v", autoKubernetesLabels)
	}

	removeKey := cfg.Get("RemoveKeys")
	if removeKey != "" {
		res.RemoveKeys = strings.Split(removeKey, ",")
	}

	labelKeys := cfg.Get("LabelKeys")
	if labelKeys != "" {
		res.LabelKeys = strings.Split(labelKeys, ",")
	}

	dropSingleKey := cfg.Get("DropSingleKey")
	switch dropSingleKey {
	case "false":
		res.DropSingleKey = false
	case "true", "":
		res.DropSingleKey = true
	default:
		return nil, fmt.Errorf("invalid boolean DropSingleKey: %v", dropSingleKey)
	}

	lineFormat := cfg.Get("LineFormat")
	switch lineFormat {
	case "json", "":
		res.LineFormat = JsonFormat
	case "key_value":
		res.LineFormat = KvPairFormat
	default:
		return nil, fmt.Errorf("invalid format: %s", lineFormat)
	}

	labelMapPath := cfg.Get("LabelMapPath")
	if labelMapPath != "" {
		content, err := ioutil.ReadFile(labelMapPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open LabelMap file: %s", err)
		}
		if err := json.Unmarshal(content, &res.LabelMap); err != nil {
			return nil, fmt.Errorf("failed to Unmarshal LabelMap file: %s", err)
		}
		res.LabelKeys = nil
	}

	labelSelector := cfg.Get("LabelSelector")
	if labelSelector != "" {
		labels := strings.Split(labelSelector, ",")
		res.LabelSelector = make(map[string]string)
		for _, label := range labels {
			splitLabel := strings.Split(label, ":")
			if len(splitLabel) != 2 {
				continue
			}
			res.LabelSelector[splitLabel[0]] = splitLabel[1]
		}
	}

	dynamicHostPath := cfg.Get("DynamicHostPath")
	if dynamicHostPath != "" {
		if err := json.Unmarshal([]byte(dynamicHostPath), &res.DynamicHostPath); err != nil {
			return nil, fmt.Errorf("failed to Unmarshal DynamicHostPath json: %s", err)
		}
	}

	res.DynamicHostPrefix = cfg.Get("DynamicHostPrefix")
	res.DynamicHostSulfix = cfg.Get("DynamicHostSulfix")
	res.DynamicHostRegex = cfg.Get("DynamicHostRegex")
	if res.DynamicHostRegex == "" {
		res.DynamicHostRegex = "*"
	}

	return res, nil
}
