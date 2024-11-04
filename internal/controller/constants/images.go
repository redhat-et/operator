package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:2d707d12e4f65e1a92b4de11465a5976d55e15ad6c9fefe994646ccd44c83840"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:7af78c7bc4df097ffeeef345f1d13289695f715221957579ee65daeef2fa3f5b"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:501612745e63e5504017079388bec191ffacf00ffdebde7be6ca5b8e4fd9d323"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:4b5765bbfd3dac5fa027d2fb3d672b6ebffbc573b9413ab4cb189c50fa6f9a09"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:18820b1fbdbc2cc3e917822974910332d937b03cfe781628bd986fd6a5ee318e"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:81e10e34f02b21bb8295e7b5c93797fc8c0e43a1a0d8304cca1b07415a3ed6f5"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:3c93c15fc5c918a91b3da9f5bf2276e4d46d881b1031287e6ab28e6aeb23e019"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:c5995c88063bd9875ae61c299bcf549002fcde724aab09807c70934e73daf356"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:79340be7918034c68a334a210ab872161827c99c2a1551a4fce8d5d27560a234"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:31e7318a9b19ed04ef0f25949f1f1709d293b532316b27a06f83fa5174547b17"

	HttpServerImage = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"

	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:1b87ff1ad02c476c08e06038a26af7abe61f177e491a9ff42d507550a8587ac8"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:fce0a22c8872309554236bab3457715dda0a83eb40dc6a9ecd3477b8023369d0"
	ClientServerImage       = "registry.redhat.io/rhtas/client-server-rhel9@sha256:e70f9862dbee9c3046ded1d93363c2183c4c9be787e4fffb860cc63b1105c799"
)
