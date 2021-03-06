package main

import (
	"io/ioutil"
	"net/url"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/weaveworks/common/logging"

	"github.com/prometheus/common/model"

	"github.com/cortexproject/cortex/pkg/util/flagext"

	"github.com/grafana/loki/pkg/promtail/client"
	lokiflag "github.com/grafana/loki/pkg/util/flagext"
)

var (
	testFileName string
	warnLogLevel logging.Level
	infoLogLevel logging.Level
	defaultURL   flagext.URLValue
	somewhereURL flagext.URLValue
)

type fakeConfig map[string]string

func (f fakeConfig) Get(key string) string {
	return f[key]
}

var _ = Describe("Config", func() {
	defer os.Remove(testFileName)

	type testArgs struct {
		conf    map[string]string
		want    *config
		wantErr bool
	}

	BeforeEach(func() {
		var err error

		testFileName, err = CreateTempLabelMap()
		Expect(err).ToNot(HaveOccurred())

		err = warnLogLevel.Set("warn")
		Expect(err).ToNot(HaveOccurred())

		err = infoLogLevel.Set("info")
		Expect(err).ToNot(HaveOccurred())

		defaultURL, err = ParseURL("http://localhost:3100/loki/api/v1/push")
		Expect(err).ToNot(HaveOccurred())

		somewhereURL, err = ParseURL("http://somewhere.com:3100/loki/api/v1/push")
		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable("Test Config",
		func(args testArgs) {
			got, err := parseConfig(fakeConfig(args.conf))
			if args.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(args.want.clientConfig.BatchSize).To(Equal(got.clientConfig.BatchSize))
				Expect(args.want.clientConfig.ExternalLabels).To(Equal(got.clientConfig.ExternalLabels))
				Expect(args.want.clientConfig.BatchWait).To(Equal(got.clientConfig.BatchWait))
				Expect(args.want.clientConfig.URL).To(Equal(got.clientConfig.URL))
				Expect(args.want.clientConfig.TenantID).To(Equal(got.clientConfig.TenantID))
				Expect(args.want.lineFormat).To(Equal(got.lineFormat))
				Expect(args.want.removeKeys).To(Equal(got.removeKeys))
				Expect(args.want.logLevel.String()).To(Equal(got.logLevel.String()))
				Expect(args.want.labelMap).To(Equal(got.labelMap))
				Expect(args.want.dynamicHostPrefix).To(Equal(got.dynamicHostPrefix))
				Expect(args.want.dynamicHostSulfix).To(Equal(got.dynamicHostSulfix))
				Expect(args.want.dynamicHostRegex).To(Equal(got.dynamicHostRegex))
			}
		},
		Entry("defaults", testArgs{
			map[string]string{},
			&config{
				lineFormat: jsonFormat,
				clientConfig: client.Config{
					URL:            defaultURL,
					BatchSize:      defaultClientCfg.BatchSize,
					BatchWait:      defaultClientCfg.BatchWait,
					ExternalLabels: lokiflag.LabelSet{LabelSet: model.LabelSet{"job": "fluent-bit"}},
				},
				logLevel:      infoLogLevel,
				dropSingleKey: true,
			},
			false},
		),
		Entry("setting values", testArgs{
			map[string]string{
				"URL":           "http://somewhere.com:3100/loki/api/v1/push",
				"TenantID":      "my-tenant-id",
				"LineFormat":    "key_value",
				"LogLevel":      "warn",
				"Labels":        `{app="foo"}`,
				"BatchWait":     "30",
				"BatchSize":     "100",
				"RemoveKeys":    "buzz,fuzz",
				"LabelKeys":     "foo,bar",
				"DropSingleKey": "false",
			},
			&config{
				lineFormat: kvPairFormat,
				clientConfig: client.Config{
					URL:            somewhereURL,
					TenantID:       "my-tenant-id",
					BatchSize:      100,
					BatchWait:      30 * time.Second,
					ExternalLabels: lokiflag.LabelSet{LabelSet: model.LabelSet{"app": "foo"}},
				},
				logLevel:      warnLogLevel,
				labelKeys:     []string{"foo", "bar"},
				removeKeys:    []string{"buzz", "fuzz"},
				dropSingleKey: false,
			},
			false},
		),
		Entry("with label map", testArgs{
			map[string]string{
				"URL":           "http://somewhere.com:3100/loki/api/v1/push",
				"LineFormat":    "key_value",
				"LogLevel":      "warn",
				"Labels":        `{app="foo"}`,
				"BatchWait":     "30",
				"BatchSize":     "100",
				"RemoveKeys":    "buzz,fuzz",
				"LabelKeys":     "foo,bar",
				"DropSingleKey": "false",
				"LabelMapPath":  testFileName,
			},
			&config{
				lineFormat: kvPairFormat,
				clientConfig: client.Config{
					URL:            somewhereURL,
					TenantID:       "", // empty as not set in fluent-bit plugin config map
					BatchSize:      100,
					BatchWait:      30 * time.Second,
					ExternalLabels: lokiflag.LabelSet{LabelSet: model.LabelSet{"app": "foo"}},
				},
				logLevel:      warnLogLevel,
				labelKeys:     nil,
				removeKeys:    []string{"buzz", "fuzz"},
				dropSingleKey: false,
				labelMap: map[string]interface{}{
					"kubernetes": map[string]interface{}{
						"container_name": "container",
						"host":           "host",
						"namespace_name": "namespace",
						"pod_name":       "instance",
						"labels": map[string]interface{}{
							"component": "component",
							"tier":      "tier",
						},
					},
					"stream": "stream",
				},
			},
			false},
		),
		Entry("with dynamic configuration", testArgs{
			map[string]string{
				"URL":               "http://somewhere.com:3100/loki/api/v1/push",
				"LineFormat":        "key_value",
				"LogLevel":          "warn",
				"Labels":            `{app="foo"}`,
				"BatchWait":         "30",
				"BatchSize":         "100",
				"RemoveKeys":        "buzz,fuzz",
				"LabelKeys":         "foo,bar",
				"DropSingleKey":     "false",
				"DynamicHostPath":   "{\"kubernetes\": {\"namespace_name\" : \"namespace\"}}",
				"DynamicHostPrefix": "http://loki.",
				"DynamicHostSulfix": ".svc:3100/loki/api/v1/push",
				"DynamicHostRegex":  "shoot--",
			},
			&config{
				lineFormat: kvPairFormat,
				clientConfig: client.Config{
					URL:            somewhereURL,
					TenantID:       "", // empty as not set in fluent-bit plugin config map
					BatchSize:      100,
					BatchWait:      30 * time.Second,
					ExternalLabels: lokiflag.LabelSet{LabelSet: model.LabelSet{"app": "foo"}},
				},
				logLevel:      warnLogLevel,
				labelKeys:     nil,
				removeKeys:    []string{"buzz", "fuzz"},
				dropSingleKey: false,
				dynamicHostPath: map[string]interface{}{
					"kubernetes": map[string]interface{}{
						"namespace_name": "namespace",
					},
				},
				dynamicHostPrefix: "http://loki.",
				dynamicHostSulfix: ".svc:3100/loki/api/v1/push",
				dynamicHostRegex:  "shoot--",
			},
			false},
		),
		Entry("bad url", testArgs{map[string]string{"URL": "::doh.com"}, nil, true}),
		Entry("bad BatchWait", testArgs{map[string]string{"BatchWait": "a"}, nil, true}),
		Entry("bad BatchSize", testArgs{map[string]string{"BatchSize": "a"}, nil, true}),
		Entry("bad labels", testArgs{map[string]string{"Labels": "a"}, nil, true}),
		Entry("bad format", testArgs{map[string]string{"LineFormat": "a"}, nil, true}),
		Entry("bad log level", testArgs{map[string]string{"LogLevel": "a"}, nil, true}),
		Entry("bad drop single key", testArgs{map[string]string{"DropSingleKey": "a"}, nil, true}),
		Entry("bad labelmap file", testArgs{map[string]string{"LabelMapPath": "a"}, nil, true}),
		Entry("bad Dynamic Host Path", testArgs{map[string]string{"DynamicHostPath": "a"}, nil, true}),
	)
})

func ParseURL(u string) (flagext.URLValue, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return flagext.URLValue{}, err
	}
	return flagext.URLValue{URL: parsed}, nil
}

func CreateTempLabelMap() (string, error) {
	file, err := ioutil.TempFile("", "labelmap")
	if err != nil {
		return "", err
	}

	_, err = file.WriteString(`{
		"kubernetes": {
			"namespace_name": "namespace",
			"labels": {
				"component": "component",
				"tier": "tier"
			},
			"host": "host",
			"container_name": "container",
			"pod_name": "instance"
		},
		"stream": "stream"
	}`)

	if err != nil {
		return "", err
	}
	return file.Name(), nil
}
