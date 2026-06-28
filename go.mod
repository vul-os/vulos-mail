module github.com/vul-os/vulos-mail

go 1.25.6

// The shared Vulos Apps & Bots platform is consumed from the sibling checkout so
// the product/proxy can host an apps & bots place. The lilmail engine stays pure
// (it never imports the platform); only this product binary does.
replace github.com/vul-os/vulos-apps => ../vulos-apps

// vulos-deliver is the shared deliverability engine (SES backend). The local
// replace lets the product build against the sibling checkout during development;
// remove it and pin a released version for production builds.
replace github.com/vul-os/vulos-deliver => ../vulos-deliver

require (
	github.com/emersion/go-imap/v2 v2.0.0-beta.8
	github.com/emersion/go-message v0.18.2
	github.com/emersion/go-msgauth v0.7.0
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.24.0
)

require golang.org/x/crypto v0.46.0

// Pinned so the module graph resolves to a release whose archive is available
// offline (the default graph would otherwise select x/net v0.47.0).
require golang.org/x/net v0.48.0 // indirect

require (
	blitiri.com.ar/go/spf v1.5.1
	filippo.io/age v1.3.1
	github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608
	github.com/emersion/go-vcard v0.0.0-20260618161152-d854b7e0e2d3
	github.com/klauspost/compress v1.18.6
	github.com/minio/minio-go/v7 v7.0.91
	github.com/prometheus/client_golang v1.23.2
	github.com/vul-os/vulos-apps v0.0.0-00010101000000-000000000000
	github.com/vul-os/vulos-deliver v0.0.0-00010101000000-000000000000
	modernc.org/sqlite v1.50.1
)

require (
	filippo.io/hpke v0.4.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/minio/crc64nvme v1.0.1 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

require (
	github.com/aws/aws-sdk-go-v2 v1.42.0 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.25 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.24 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/sesv2 v1.62.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.31.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.43.3 // indirect
	github.com/aws/smithy-go v1.27.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/text v0.32.0 // indirect
)
