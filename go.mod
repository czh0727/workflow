module git.sotatts.online/matrix/matrix/workflow/api-server

go 1.26.1

replace git.sotatts.online/matrix/matrix/packages/backend/go/nacos v0.1.0 => ./third_party/backend/go/nacos

replace git.sotatts.online/matrix/matrix/packages/backend/go/httpclient v0.1.0 => ./third_party/backend/go/httpclient

replace git.sotatts.online/matrix/matrix/packages/backend/go/obs v0.1.0 => ./third_party/backend/go/obs

require (
	git.sotatts.online/matrix/matrix/packages/backend/go/httpclient v0.1.0
	git.sotatts.online/matrix/matrix/packages/backend/go/nacos v0.1.0
	git.sotatts.online/matrix/matrix/packages/backend/go/obs v0.1.0
	github.com/expr-lang/expr v1.17.8
	github.com/go-viper/mapstructure/v2 v2.5.0
	github.com/google/gnostic v0.7.1
	github.com/google/uuid v1.6.0
	github.com/google/wire v0.6.0
	github.com/jackc/pgerrcode v0.0.0-20250907135507-afb5586c32a6
	github.com/jackc/pgx/v5 v5.10.0
	github.com/prometheus/client_golang v1.23.2
	github.com/stretchr/testify v1.11.1
	go.einride.tech/aip v0.86.3
	go.opentelemetry.io/otel/exporters/prometheus v0.66.0
	go.opentelemetry.io/otel/sdk/metric v1.45.0
	go.temporal.io/api v1.63.4
	go.temporal.io/sdk v1.47.0
	go.temporal.io/sdk/contrib/opentelemetry v0.8.1
	go.uber.org/automaxprocs v1.6.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260803160001-6ac0973c030d
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/alibabacloud-go/debug v1.0.1 // indirect
	github.com/alibabacloud-go/tea v1.5.3 // indirect
	github.com/alibabacloud-go/tea-utils v1.4.4 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v1.63.107 // indirect
	github.com/aliyun/alibabacloud-dkms-gcs-go-sdk v0.2.2 // indirect
	github.com/aliyun/alibabacloud-dkms-transfer-go-sdk v0.1.7 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.2.0 // indirect
	github.com/bytedance/gopkg v0.1.4 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/gin-contrib/sse v1.1.1 // indirect
	github.com/gin-gonic/gin v1.12.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.7.0-rc.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.21 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nacos-group/nacos-sdk-go/v2 v2.2.7 // indirect
	github.com/nexus-rpc/nexus-proto-annotations v0.1.0 // indirect
	github.com/nexus-rpc/sdk-go v0.6.0 // indirect
	github.com/opentracing/opentracing-go v1.2.1-0.20220228012449-10b1cf09e00b // indirect
	github.com/pelletier/go-toml/v2 v2.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.45.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.45.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/arch v0.25.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto v0.0.0-20260128011058-8636f8732409 // indirect
	gopkg.in/ini.v1 v1.67.2 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

require (
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-kratos/kratos/contrib/otel/v3 v3.0.0-20260515082355-1ddb58e407c5
	github.com/go-kratos/kratos/v3 v3.0.0
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/form/v4 v4.3.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	go.opentelemetry.io/otel v1.45.0
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260803160001-6ac0973c030d // indirect
	gopkg.in/yaml.v3 v3.0.1
)
