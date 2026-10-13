package conf

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

type Server struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type Log struct {
	Level string `mapstructure:"level"`
}

type Database struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

// Sidecar 管 Zen 侧车进程：二进制路径、API 与管理口监听地址、配置文件路径。
// 侧车默认关闭，无启用开关：--o2a 或面板开关仅影响本轮运行。
type Sidecar struct {
	Binary    string `mapstructure:"binary"`
	APIAddr   string `mapstructure:"api_addr"`
	AdminAddr string `mapstructure:"admin_addr"`
	Config    string `mapstructure:"config"`
}

type Config struct {
	Server   Server   `mapstructure:"server"`
	Log      Log      `mapstructure:"log"`
	Database Database `mapstructure:"database"`
	Sidecar  Sidecar  `mapstructure:"sidecar"`
}

var AppConfig Config

func Load(path string) error {
	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("json")
		viper.AddConfigPath("data")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix(APP_NAME)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults()

	if err := viper.ReadInConfig(); err == nil {
		log.Infof("Using config file: %s", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Infof("Config file not found, creating default config")
			if err := os.MkdirAll("data", 0755); err != nil {
				log.Errorf("Failed to create data directory: %v", err)
			}
			if err := viper.SafeWriteConfigAs("data/config.json"); err != nil {
				log.Errorf("Failed to create default config: %v", err)
			}
		} else {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		return fmt.Errorf("unable to decode config into struct: %w", err)
	}
	return nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("database.type", "sqlite")
	viper.SetDefault("database.path", "data/data.db")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("sidecar.binary", "data/sidecar/opencode2api.exe")
	viper.SetDefault("sidecar.api_addr", "127.0.0.1:8090")
	viper.SetDefault("sidecar.admin_addr", "127.0.0.1:8091")
	viper.SetDefault("sidecar.config", "data/sidecar/sidecar.json")
}
