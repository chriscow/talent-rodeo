module talent-rodeo

go 1.23.0

replace github.com/thoughtnet/minds => ../thoughtnet/minds/

replace github.com/thoughtnet/minds/openai => ../thoughtnet/minds/openai

replace github.com/thoughtnet/minds/gemini => ../thoughtnet/minds/gemini

replace github.com/thoughtnet/minds/tools => ../thoughtnet/minds/tools

require (
	github.com/alexedwards/flow v0.1.0
	github.com/go-playground/form/v4 v4.2.1
	github.com/golang-migrate/migrate/v4 v4.18.1
	github.com/gorilla/sessions v1.4.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/lmittmann/tint v1.0.6
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/spf13/cobra v1.8.1
	github.com/thoughtnet/minds v1.0.0
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce
	github.com/wneessen/go-mail v0.5.2
	golang.org/x/crypto v0.31.0
	golang.org/x/exp v0.0.0-20241217172543-b2144cdd0a67
	golang.org/x/text v0.21.0
)

require (
	github.com/WqyJh/go-openai-realtime v0.3.4 // indirect
	github.com/coder/websocket v1.8.12 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/net v0.32.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
