module talent-rodeo

go 1.23.3

toolchain go1.23.4

replace github.com/chriscow/minds => ../thoughtnet/minds/

replace github.com/chriscow/minds/openai => ../thoughtnet/minds/openai

replace github.com/chriscow/minds/gemini => ../thoughtnet/minds/gemini

replace github.com/chriscow/minds/tools => ../thoughtnet/minds/tools

require (
	github.com/WqyJh/go-openai-realtime v0.3.4
	github.com/a-h/templ v0.2.793
	github.com/alexedwards/flow v0.1.0
	github.com/chriscow/minds v0.0.0
	github.com/delaneyj/toolbelt v0.3.15
	github.com/go-playground/form/v4 v4.2.1
	github.com/golang-migrate/migrate/v4 v4.18.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/pat v0.0.0-20180118222023-199c85a7f6d1
	github.com/gorilla/sessions v1.4.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/joho/godotenv v1.5.1
	github.com/justinas/nosurf v1.1.1
	github.com/lib/pq v1.10.9
	github.com/lmittmann/tint v1.0.6
	github.com/markbates/goth v1.80.0
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/nats-io/nats-server/v2 v2.10.24
	github.com/nats-io/nats.go v1.37.0
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/spf13/cobra v1.8.1
	github.com/starfederation/datastar v0.21.3
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce
	github.com/wneessen/go-mail v0.5.2
	golang.org/x/crypto v0.31.0
	golang.org/x/exp v0.0.0-20241217172543-b2144cdd0a67
	golang.org/x/text v0.21.0
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/CAFxX/httpcompression v0.0.9 // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/chewxy/math32 v1.11.1 // indirect
	github.com/coder/websocket v1.8.12 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/delaneyj/gostar v0.8.0 // indirect
	github.com/denisbrodbeck/machineid v1.0.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-rod/rod v0.116.2 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/golang-jwt/jwt/v4 v4.4.2 // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/igrmk/treemap/v2 v2.0.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx v1.2.29 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/markbates/going v1.0.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/mrjones/oauth v0.0.0-20180629183705-f4e24b6d100c // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rzajac/clock v0.2.0 // indirect
	github.com/rzajac/zflake v0.8.0 // indirect
	github.com/samber/lo v1.47.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/ysmood/fetchup v0.2.4 // indirect
	github.com/ysmood/goob v0.4.0 // indirect
	github.com/ysmood/got v0.40.0 // indirect
	github.com/ysmood/gson v0.7.3 // indirect
	github.com/ysmood/leakless v0.9.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/net v0.32.0 // indirect
	golang.org/x/oauth2 v0.24.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	google.golang.org/protobuf v1.35.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	modernc.org/libc v1.61.2 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.34.1 // indirect
	zombiezen.com/go/sqlite v1.4.0 // indirect
)
