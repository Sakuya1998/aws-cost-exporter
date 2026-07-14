package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Options describes configuration sources supplied by the process entry point.
type Options struct {
	Path      string
	Overrides map[string]any
}

// Load merges defaults, YAML, environment, and caller overrides in increasing
// precedence order.
func Load(options Options) (Config, error) {
	source := viper.New()
	source.SetConfigType("yaml")
	source.SetEnvPrefix("AWS_COST_EXPORTER")
	source.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	source.AutomaticEnv()
	registerDefaults(source, "", reflect.ValueOf(Default()))

	if options.Path != "" {
		source.SetConfigFile(options.Path)
		if err := source.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}
	for key, value := range options.Overrides {
		source.Set(key, value)
	}

	var result Config
	if err := source.UnmarshalExact(
		&result,
		viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc()),
	); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := Validate(result); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return result, nil
}

// Check verifies that all configured sources can be loaded.
func Check(options Options) error {
	_, err := Load(options)

	return err
}

// registerDefaults recursively registers schema fields so AutomaticEnv can
// discover environment-only overrides.
func registerDefaults(source *viper.Viper, prefix string, value reflect.Value) {
	valueType := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldType := valueType.Field(index)
		key := fieldType.Tag.Get("mapstructure")
		if prefix != "" {
			key = prefix + "." + key
		}

		field := value.Field(index)
		if field.Kind() == reflect.Struct {
			registerDefaults(source, key, field)
			continue
		}
		source.SetDefault(key, field.Interface())
	}
}
