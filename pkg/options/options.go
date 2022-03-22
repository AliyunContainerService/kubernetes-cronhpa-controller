package options

var (
	Cfg = &Config{}
)

type Config struct {
	EnableNotify bool
	Webhook      string
}

func GlobalConfiguration() *Config {
	return Cfg
}

func InitializationConfigWithOptions(opts ...func(config *Config)) {
	for _, opt := range opts {
		opt(Cfg)
	}
}

func SetWebhook(url string) func(config *Config) {
	return func(config *Config) {
		config.Webhook = url
	}
}

func EnableNotify(enable bool) func(config *Config) {
	return func(config *Config) {
		config.EnableNotify = enable
	}
}
