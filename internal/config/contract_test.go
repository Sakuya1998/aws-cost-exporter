package config

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var updateContract = flag.Bool("update-contract", false, "rewrite the reviewed v1 contract fixture")

type configFieldContract struct {
	Path       string `json:"path"`
	GoType     string `json:"go_type"`
	Default    string `json:"default,omitempty"`
	Collection bool   `json:"collection,omitempty"`
}

type configContract struct {
	Version  string                `json:"version"`
	TopLevel []string              `json:"top_level"`
	Fields   []configFieldContract `json:"fields"`
}

func TestV1ConfigurationContract(t *testing.T) {
	contract := buildV1ConfigurationContract(t)
	wantTopLevel := []string{"server", "log", "aws", "targets", "collection", "cache", "telemetry"}
	if !reflect.DeepEqual(contract.TopLevel, wantTopLevel) {
		t.Fatalf("top-level configuration = %v, want %v", contract.TopLevel, wantTopLevel)
	}

	got, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatalf("marshal v1 configuration contract: %v", err)
	}

	fixturePath := filepath.Join("testdata", "v1", "config-contract.json")
	if *updateContract {
		if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
			t.Fatalf("create contract fixture directory: %v", err)
		}
		if err := os.WriteFile(fixturePath, got, 0o600); err != nil {
			t.Fatalf("write contract fixture: %v", err)
		}
	}

	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read reviewed v1 contract fixture: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("v1 configuration contract changed; review the change, then run go test ./internal/config -run TestV1ConfigurationContract -args -update-contract\n%s", contractDiff(want, got))
	}
}

func TestV1ConfigurationContractExampleLoadsAndValidates(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "aws-cost-exporter.example.yaml")
	value, err := Load(Options{Path: path})
	if err != nil {
		t.Fatalf("Load(%s) = %v", path, err)
	}
	if err := Validate(value); err != nil {
		t.Fatalf("Validate(Load(%s)) = %v", path, err)
	}
}

func TestV1ConfigurationContractRejectsLegacyScheduler(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	document := []byte("scheduler:\n  max_concurrency: 2\naws:\n  credentials:\n    sources:\n      runtime:\n        type: default_chain\ntargets:\n  - name: payer-prod\n    account_id: \"444455556666\"\n    required: true\n    credentials:\n      source: runtime\n    cost_explorer:\n      enabled: true\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(Options{Path: path})
	if err == nil || !strings.Contains(err.Error(), "decode config") || !strings.Contains(err.Error(), "scheduler") {
		t.Fatalf("Load(legacy scheduler) = %v, want field-specific scheduler error", err)
	}
}

func buildV1ConfigurationContract(t *testing.T) configContract {
	t.Helper()
	contract := configContract{Version: "v1.0.0"}
	configType := reflect.TypeOf(Config{})
	defaults := reflect.ValueOf(Default())
	for index := 0; index < configType.NumField(); index++ {
		field := configType.Field(index)
		if !field.IsExported() {
			continue
		}
		name := requireConfigurationTags(t, configType, field)
		contract.TopLevel = append(contract.TopLevel, name)
	}
	collectConfigurationFields(t, configType, defaults, "", true, &contract.Fields)
	sort.Slice(contract.Fields, func(i, j int) bool {
		return contract.Fields[i].Path < contract.Fields[j].Path
	})
	return contract
}

func collectConfigurationFields(t *testing.T, valueType reflect.Type, defaults reflect.Value, prefix string, hasDefaults bool, fields *[]configFieldContract) {
	t.Helper()
	for index := 0; index < valueType.NumField(); index++ {
		field := valueType.Field(index)
		if !field.IsExported() {
			continue
		}
		name := requireConfigurationTags(t, valueType, field)
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		fieldDefaults := reflect.Zero(field.Type)
		if hasDefaults {
			fieldDefaults = defaults.Field(index)
		}
		kind := field.Type.Kind()
		*fields = append(*fields, configFieldContract{
			Path:       path,
			GoType:     field.Type.String(),
			Default:    normalizeConfigurationDefault(t, fieldDefaults, hasDefaults),
			Collection: kind == reflect.Array || kind == reflect.Map || kind == reflect.Slice,
		})

		childType, childDefaults, childPath, childHasDefaults, ok := configurationChild(field.Type, fieldDefaults, path, hasDefaults)
		if ok {
			collectConfigurationFields(t, childType, childDefaults, childPath, childHasDefaults, fields)
		}
	}
}

func requireConfigurationTags(t *testing.T, owner reflect.Type, field reflect.StructField) string {
	t.Helper()
	yamlName := field.Tag.Get("yaml")
	mapstructureName := field.Tag.Get("mapstructure")
	if yamlName == "" || mapstructureName == "" || yamlName != mapstructureName {
		t.Fatalf("%s.%s must have matching non-empty yaml and mapstructure tags, got yaml=%q mapstructure=%q", owner.Name(), field.Name, yamlName, mapstructureName)
	}
	return yamlName
}

func configurationChild(valueType reflect.Type, defaults reflect.Value, path string, hasDefaults bool) (reflect.Type, reflect.Value, string, bool, bool) {
	switch valueType.Kind() {
	case reflect.Struct:
		return valueType, defaults, path, hasDefaults, true
	case reflect.Pointer:
		if valueType.Elem().Kind() != reflect.Struct {
			return nil, reflect.Value{}, "", false, false
		}
		if hasDefaults && !defaults.IsNil() {
			return valueType.Elem(), defaults.Elem(), path, true, true
		}
		return valueType.Elem(), reflect.Zero(valueType.Elem()), path, false, true
	case reflect.Array, reflect.Slice:
		if valueType.Elem().Kind() != reflect.Struct {
			return nil, reflect.Value{}, "", false, false
		}
		return valueType.Elem(), reflect.Zero(valueType.Elem()), path + "[]", false, true
	case reflect.Map:
		if valueType.Elem().Kind() != reflect.Struct {
			return nil, reflect.Value{}, "", false, false
		}
		return valueType.Elem(), reflect.Zero(valueType.Elem()), path + ".*", false, true
	default:
		return nil, reflect.Value{}, "", false, false
	}
}

func normalizeConfigurationDefault(t *testing.T, value reflect.Value, hasDefault bool) string {
	t.Helper()
	if !hasDefault || !value.IsValid() || value.IsZero() {
		return ""
	}
	if value.Type() == reflect.TypeOf(time.Duration(0)) {
		return value.Interface().(time.Duration).String()
	}
	switch value.Kind() {
	case reflect.String:
		return value.String()
	case reflect.Bool:
		return strconv.FormatBool(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(value.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'g', -1, value.Type().Bits())
	case reflect.Array, reflect.Map, reflect.Slice:
		encoded, err := json.Marshal(value.Interface())
		if err != nil {
			t.Fatalf("normalize default for %s: %v", value.Type(), err)
		}
		return string(encoded)
	default:
		return ""
	}
}

func contractDiff(want, got []byte) string {
	return fmt.Sprintf("want:\n%s\n\ngot:\n%s", want, got)
}
