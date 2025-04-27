module talent-rodeo

go 1.23.3

toolchain go1.23.4

replace (
	github.com/chriscow/go-openai-realtime => ../go-openai-realtime
	github.com/chriscow/minds => ../thoughtnet/minds
	github.com/chriscow/minds/providers/gemini => ../thoughtnet/minds/providers/gemini
	github.com/chriscow/minds/providers/openai => ../thoughtnet/minds/providers/openai
	github.com/chriscow/minds/tools => ../thoughtnet/minds/tools
	github.com/chriscow/vapi-go => ../vapi-go
)

require (
	cloud.google.com/go/firestore v1.18.0
	firebase.google.com/go/v4 v4.15.2
	github.com/ThreeDotsLabs/watermill v1.4.4
	github.com/ThreeDotsLabs/watermill-nats/v2 v2.1.2
	github.com/alexedwards/flow v0.1.0
	github.com/chriscow/minds v0.0.8-0.20250303171538-dc7c074db869
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
	github.com/mattn/go-sqlite3 v1.14.24 // indirect
	github.com/nats-io/nats-server/v2 v2.10.24
	github.com/nats-io/nats.go v1.37.0
	github.com/pusher/pusher-http-go/v5 v5.1.1
	github.com/spf13/cobra v1.8.1
	github.com/tmaxmax/go-sse v0.10.0
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce
	github.com/twilio/twilio-go v1.23.11
	github.com/wneessen/go-mail v0.5.2
	golang.org/x/crypto v0.38.0
	golang.org/x/exp v0.0.0-20241217172543-b2144cdd0a67
	golang.org/x/oauth2 v0.28.0
	golang.org/x/text v0.25.0
	google.golang.org/api v0.228.0
	google.golang.org/grpc v1.72.0
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
	github.com/WqyJh/go-openai-realtime v0.5.0
	github.com/alexedwards/scs/firestore v0.0.0-20250212122300-421ef1d8611c
	github.com/alexedwards/scs/v2 v2.8.0
	github.com/cheggaaa/pb v1.0.29
	github.com/chriscow/minds/providers/gemini v0.0.5-0.20250303171538-dc7c074db869
	github.com/chriscow/minds/providers/openai v0.0.5
	github.com/chriscow/minds/tools v0.0.3
	github.com/chriscow/vapi-go v0.0.0-20250420143011-0a8970d55b72
	github.com/cloudflare/cloudflare-go v0.115.0
	github.com/dgraph-io/ristretto/v2 v2.2.0
	github.com/getsentry/sentry-go v0.32.0
	github.com/ledongthuc/pdf v0.0.0-20240201131950-da5b75280b06
	github.com/pinecone-io/go-pinecone/v3 v3.1.0
	github.com/playwright-community/playwright-go v0.5101.0
	github.com/samber/lo v1.50.0
	github.com/sashabaranov/go-openai v1.39.1
	github.com/stretchr/testify v1.10.0
)

require (
	cel.dev/expr v0.20.0 // indirect
	cloud.google.com/go v0.118.0 // indirect
	cloud.google.com/go/ai v0.10.0 // indirect
	cloud.google.com/go/auth v0.15.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	cloud.google.com/go/iam v1.2.2 // indirect
	cloud.google.com/go/longrunning v0.6.4 // indirect
	cloud.google.com/go/monitoring v1.21.2 // indirect
	cloud.google.com/go/storage v1.49.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.26.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.48.1 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.48.1 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20250121191232-2f005788dc42 // indirect
	github.com/coder/websocket v1.8.12 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.32.4 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fumiama/imgsz v0.0.2 // indirect
	github.com/go-jose/go-jose/v3 v3.0.4 // indirect
	github.com/go-jose/go-jose/v4 v4.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/generative-ai-go v0.19.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/lithammer/shortuuid/v3 v3.0.7 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/oapi-codegen/runtime v1.1.1 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spiffe/go-spiffe/v2 v2.5.0 // indirect
	github.com/zeebo/errs v1.4.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.34.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.59.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/appengine/v2 v2.0.6 // indirect
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250505200425-f936aa4a68b2 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250505200425-f936aa4a68b2 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
