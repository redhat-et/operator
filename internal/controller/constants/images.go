package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:8b85bebbaaf59dcbd0fc8bf362382841b1d6700371763660bbe63452c619acc2"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:6afced48a9e972617e8dabaf9b4e17219ece6861f6e725f76bf1b72da5d83d3e"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:d2878a957fa67eb1b20f1fd2837cd1d6b2d670f79b139f751e18236ec08c2e2e"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:ecf413ff0db920ebb03189171c4a71c2c004a2dd69551f592bc5c409ea24267d"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:18820b1fbdbc2cc3e917822974910332d937b03cfe781628bd986fd6a5ee318e"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:81e10e34f02b21bb8295e7b5c93797fc8c0e43a1a0d8304cca1b07415a3ed6f5"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:3c93c15fc5c918a91b3da9f5bf2276e4d46d881b1031287e6ab28e6aeb23e019"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:6aa3ca40e0f9e32a0a211a930b21ff009b83e46609bfa5bb328979e4799d13c7"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:5f926ac1faee5defe495bddab3c36adb97790cbbc5fc1a48827427edf94f50b9"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:31e7318a9b19ed04ef0f25949f1f1709d293b532316b27a06f83fa5174547b17"

	HttpServerImage = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"

	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:1b87ff1ad02c476c08e06038a26af7abe61f177e491a9ff42d507550a8587ac8"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:cd5949f18df0fea83b6ba37041c7eba7d296ff2329d3e2c985812951e4238d52"
	ClientServerImage       = "registry.redhat.io/rhtas/client-server-rhel9@sha256:9537329d0166b8d41ffd5f5d79c052fc27abe426a20cba5733c84030013c4e29"
)
