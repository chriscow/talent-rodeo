module talent-rodeo

go 1.23.3

toolchain go1.23.4

replace github.com/chriscow/minds/providers/openai => ../thoughtnet/minds/providers/openai

replace github.com/chriscow/minds/providers/gemini => ../thoughtnet/minds/providers/gemini

replace github.com/chriscow/minds => ../thoughtnet/minds

replace github.com/chriscow/minds/tools => ../thoughtnet/minds/tools

replace github.com/WqyJh/go-openai-realtime => /Users/chris/github.com/go-openai-realtime

require (
	github.com/WqyJh/go-openai-realtime v0.0.0-00010101000000-000000000000
	github.com/alexedwards/flow v0.1.0
	github.com/chriscow/minds v0.0.7
	github.com/chriscow/minds/providers/gemini v0.0.0-00010101000000-000000000000
	github.com/chriscow/minds/providers/openai v0.0.0-00010101000000-000000000000
	github.com/delaneyj/toolbelt v0.3.15
	github.com/fatih/color v1.16.0
	github.com/fumiama/go-docx v0.0.0-20241231153056-9f8f327c74a5
	github.com/go-playground/form/v4 v4.2.1
	github.com/golang-migrate/migrate/v4 v4.18.1
	github.com/gomarkdown/markdown v0.0.0-20241205020045-f7e15b2f3e62
	github.com/google/uuid v1.6.0
	github.com/gorilla/sessions v1.4.0
	github.com/gorilla/websocket v1.5.3
	github.com/jmoiron/sqlx v1.4.0
	github.com/joho/godotenv v1.5.1
	github.com/justinas/nosurf v1.1.1
	github.com/lib/pq v1.10.9
	github.com/lmittmann/tint v1.0.6
	github.com/markbates/goth v1.80.0
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/nats-io/nats-server/v2 v2.10.24
	github.com/nats-io/nats.go v1.37.0
	github.com/pusher/pusher-http-go/v5 v5.1.1
	github.com/spf13/cobra v1.8.1
	github.com/tmaxmax/go-sse v0.10.0
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce
	github.com/twilio/twilio-go v1.23.9
	github.com/wneessen/go-mail v0.5.2
	golang.org/x/crypto v0.32.0
	golang.org/x/exp v0.0.0-20241217172543-b2144cdd0a67
	golang.org/x/oauth2 v0.25.0
	golang.org/x/text v0.21.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tiktoken-go/tokenizer v0.2.1 // indirect
)

require (
	cloud.google.com/go v0.118.0 // indirect
	cloud.google.com/go/ai v0.10.0 // indirect
	cloud.google.com/go/auth v0.14.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.7 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	cloud.google.com/go/longrunning v0.6.4 // indirect
	github.com/CAFxX/httpcompression v0.0.9 // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/chewxy/math32 v1.11.1 // indirect
	github.com/coder/websocket v1.8.12 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/denisbrodbeck/machineid v1.0.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fumiama/imgsz v0.0.2 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-rod/rod v0.116.2 // indirect
	github.com/google/generative-ai-go v0.19.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rzajac/clock v0.2.0 // indirect
	github.com/rzajac/zflake v0.8.0 // indirect
	github.com/sashabaranov/go-openai v1.36.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/ysmood/fetchup v0.2.4 // indirect
	github.com/ysmood/goob v0.4.0 // indirect
	github.com/ysmood/got v0.40.0 // indirect
	github.com/ysmood/gson v0.7.3 // indirect
	github.com/ysmood/leakless v0.9.0 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.59.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/time v0.9.0 // indirect
	google.golang.org/api v0.217.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/grpc v1.69.4 // indirect
	google.golang.org/protobuf v1.36.3 // indirect
	modernc.org/libc v1.61.2 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.34.1 // indirect
	zombiezen.com/go/sqlite v1.4.0 // indirect
)
