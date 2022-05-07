package bot

import (
	"fmt"
	"github.com/spf13/viper"
	"time"
)

type BaseConfig struct {
	Mnemonic         string        `mapstructure:"mnemonic"`
	PublicKey        string        `mapstructure:"pub-key"`
	PrivateKey       string        `mapstructure:"priv-key"`
	TransferAmount   uint64        `mapstructure:"transfer-amount"`
	Server           string        `mapstructure:"server"`
	BotToken         string        `mapstructure:"token"`
	RequestCoolDown  time.Duration `mapstructure:"cooldown"`
	SecureConnection bool          `mapstructure:"secure"`
}

func DefaultConfig() *BaseConfig {
	return &BaseConfig{}
}

const defaultConfigFileName = "config.toml"

// LoadConfigFromFile tries to load configuration file if the config parameter was specified
func LoadConfigFromFile() (*BaseConfig, error) {
	fileLocation := viper.GetString("config")
	vip := viper.New()
	// read in default config if passed as param using viper
	if err := LoadConfig(fileLocation, vip); err != nil {
		fmt.Println(fmt.Sprintf("couldn't load config file at location: %s switching to defaults \n error: %v.",
			fileLocation, err))
		// return err
	}

	conf := DefaultConfig()
	// load config if it was loaded to our viper
	err := vip.Unmarshal(&conf)
	if err != nil {
		fmt.Println(fmt.Sprintf("Failed to parse config %v", err))

		return nil, err
	}
	return conf, nil
}

// LoadConfig load the config file
func LoadConfig(fileLocation string, vip *viper.Viper) (err error) {
	if fileLocation == "" {
		fileLocation = defaultConfigFileName
	}

	vip.SetConfigFile(fileLocation)
	err = vip.ReadInConfig()

	if err != nil {
		if fileLocation != defaultConfigFileName {
			fmt.Sprintf("failed loading config from %v trying %v. error %v", fileLocation, defaultConfigFileName, err)
			fmt.Println(fmt.Sprintf("Failed to parse config %v", err))
			vip.SetConfigFile(defaultConfigFileName)
			err = vip.ReadInConfig()
		}
		// we change err so check again
		if err != nil {
			return fmt.Errorf("failed to read config file %v", err)
		}
	}

	return nil
}
