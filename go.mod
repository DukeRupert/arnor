module github.com/dukerupert/arnor

go 1.25.4

require (
	github.com/dukerupert/fornost v0.0.0-00010101000000-000000000000
	github.com/dukerupert/gwaihir v0.0.0-00010101000000-000000000000
	github.com/dukerupert/shadowfax v0.0.0-00010101000000-000000000000
	github.com/joho/godotenv v1.5.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.20.1
	golang.org/x/crypto v0.32.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
)

replace (
	github.com/dukerupert/fornost => ../fornost
	github.com/dukerupert/gwaihir => ../gwaihir
	github.com/dukerupert/shadowfax => ../shadowfax
)
