package lokiplugin

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logfmt/logfmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"

	"github.com/gardener/logging/fluent-bit-to-loki/pkg/config"
)

// prevent base64-encoding []byte values (default json.Encoder rule) by
// converting them to strings

func toStringSlice(slice []interface{}) []interface{} {
	var s []interface{}
	for _, v := range slice {
		switch t := v.(type) {
		case []byte:
			s = append(s, string(t))
		case map[interface{}]interface{}:
			s = append(s, toStringMap(t))
		case []interface{}:
			s = append(s, toStringSlice(t))
		default:
			s = append(s, t)
		}
	}
	return s
}

func toStringMap(record map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range record {
		key, ok := k.(string)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case []byte:
			m[key] = string(t)
		case map[interface{}]interface{}:
			m[key] = toStringMap(t)
		case []interface{}:
			m[key] = toStringSlice(t)
		default:
			m[key] = v
		}
	}

	return m
}

func autoLabels(records map[string]interface{}, kuberneteslbs model.LabelSet) error {
	kube, ok := records["kubernetes"]
	if !ok {
		return errors.New("kubernetes labels not found, no labels will be added")
	}

	replacer := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	for k, v := range kube.(map[string]interface{}) {
		switch k {
		case "labels":
			for m, n := range v.(map[string]interface{}) {
				kuberneteslbs[model.LabelName(replacer.Replace(m))] = model.LabelValue(fmt.Sprintf("%v", n))
			}
		case "docker_id", "pod_id", "annotations":
			// do nothing
			continue
		default:
			kuberneteslbs[model.LabelName(k)] = model.LabelValue(fmt.Sprintf("%v", v))
		}
	}

	return nil
}

func extractLabels(records map[string]interface{}, keys []string) model.LabelSet {
	res := model.LabelSet{}
	for _, k := range keys {
		v, ok := records[k]
		if !ok {
			continue
		}
		ln := model.LabelName(k)
		// skips invalid name and values
		if !ln.IsValid() {
			continue
		}
		lv := model.LabelValue(fmt.Sprintf("%v", v))
		if !lv.IsValid() {
			continue
		}
		res[ln] = lv
	}
	return res
}

// mapLabels convert records into labels using a json map[string]interface{} mapping
func mapLabels(records map[string]interface{}, mapping map[string]interface{}, res model.LabelSet) {
	for k, v := range mapping {
		switch nextKey := v.(type) {
		// if the next level is a map we are expecting we need to move deeper in the tree
		case map[string]interface{}:
			if nextValue, ok := records[k].(map[string]interface{}); ok {
				// recursively search through the next level map.
				mapLabels(nextValue, nextKey, res)
			}
		// we found a value in the mapping meaning we need to save the corresponding record value for the given key.
		case string:
			if value, ok := getRecordValue(k, records); ok {
				lName := model.LabelName(nextKey)
				lValue := model.LabelValue(value)
				if lValue.IsValid() && lName.IsValid() {
					res[lName] = lValue
				}
			}
		}
	}
}

func getDynamicHostName(records map[string]interface{}, mapping map[string]interface{}) string {
	for k, v := range mapping {
		switch nextKey := v.(type) {
		// if the next level is a map we are expecting we need to move deeper in the tree
		case map[string]interface{}:
			if nextValue, ok := records[k].(map[string]interface{}); ok {
				// recursively search through the next level map.
				return getDynamicHostName(nextValue, nextKey)
			}
		case string:
			if value, ok := getRecordValue(k, records); ok {
				return value
			}
		}
	}
	return ""
}

func getRecordValue(key string, records map[string]interface{}) (string, bool) {
	if value, ok := records[key]; ok {
		switch typedVal := value.(type) {
		case string:
			return typedVal, true
		case []byte:
			return string(typedVal), true
		default:
			return fmt.Sprintf("%v", typedVal), true
		}
	}
	return "", false
}

func removeKeys(records map[string]interface{}, keys []string) {
	for _, k := range keys {
		delete(records, k)
	}
}

func createLine(records map[string]interface{}, f config.Format) (string, error) {
	switch f {
	case config.JsonFormat:
		js, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(records)
		if err != nil {
			return "", err
		}
		return string(js), nil
	case config.KvPairFormat:
		buf := &bytes.Buffer{}
		enc := logfmt.NewEncoder(buf)
		keys := make([]string, 0, len(records))
		for k := range records {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			err := enc.EncodeKeyval(k, records[k])
			if err == logfmt.ErrUnsupportedValueType {
				err := enc.EncodeKeyval(k, fmt.Sprintf("%+v", records[k]))
				if err != nil {
					return "", nil
				}
				continue
			}
			if err != nil {
				return "", nil
			}
		}
		return buf.String(), nil
	default:
		return "", fmt.Errorf("invalid line format: %v", f)
	}
}
