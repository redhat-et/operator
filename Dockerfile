# Build the manager binary
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.21@sha256:98a0ff138c536eee98704d6909699ad5d0725a20573e2c510a60ef462b45cce0 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY client/ client/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN go mod download && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -mod=readonly -a -o manager main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal@sha256:582e18f13291d7c686ec4e6e92d20b24c62ae0fc72767c46f30a69b1a6198055
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

LABEL description="The image for the rhtas-operator."
LABEL io.k8s.description="The image for the rhtas-operator."
LABEL io.k8s.display-name="RHTAS operator container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="rhtas-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator for the rhtas-operator."
LABEL com.redhat.component="rhtas-operator"

LABEL features.operators.openshift.io/cni="false"
LABEL features.operators.openshift.io/disconnected="false"
LABEL features.operators.openshift.io/disconnected="false"
LABEL features.operators.openshift.io/fips-compliant="false"
LABEL features.operators.openshift.io/proxy-aware="false"
LABEL features.operators.openshift.io/cnf="false"
LABEL features.operators.openshift.io/cni="false"
LABEL features.operators.openshift.io/csi="false"
LABEL features.operators.openshift.io/tls-profiles="false"
LABEL features.operators.openshift.io/token-auth-aws="false"
LABEL features.operators.openshift.io/token-auth-azure="false"
LABEL features.operators.openshift.io/token-auth-gcp="false"

ENTRYPOINT ["/manager"]
