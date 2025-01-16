package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:8b85bebbaaf59dcbd0fc8bf362382841b1d6700371763660bbe63452c619acc2"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:6afced48a9e972617e8dabaf9b4e17219ece6861f6e725f76bf1b72da5d83d3e"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:d2878a957fa67eb1b20f1fd2837cd1d6b2d670f79b139f751e18236ec08c2e2e"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:ecf413ff0db920ebb03189171c4a71c2c004a2dd69551f592bc5c409ea24267d"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:78d946d1182b0d3837097de826b6b2d3f89891e0812795571adfb51194e2c469"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:2fd07c321e0a8e859d43580d060c54e453e0ba2c6f2152de9721952a852dd7d0"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:ab85e4f3fe88f7c6a376445273ce5b76c10dc805e438314fbab6d668e75ed53d"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:4ab47035967b86b84a864bb1c64fa16f71d00a3edd7d293743ec036d005740eb"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:8495aca9de2d20811acf226fcc9a6730edb78166b2aac03516bda5e32063d5a3"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:9872e1a63aed2317dc2353078824d9bfc61f833521d00de1ad41259b0b69f8b5"

	HttpServerImage = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"

	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:d8e65cbeb56bcc0a2ff0cd3afea3d35448695787835fe44c85a8991e6920bdb5"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:cd5949f18df0fea83b6ba37041c7eba7d296ff2329d3e2c985812951e4238d52"
	ClientServerImage       = "registry.redhat.io/rhtas/client-server-rhel9@sha256:3c347538e04b876d022a084caf2bd36986ad140f800d0b02c2bbec3cc534974d"
)
