# RHTAS operator
The RHTAS(Red Hat Trusted Artifact Signer) operator allows for the deployment of a production ready version of the SigStore project.

## Description
Red Hat Trusted Artifact Signer enhances software supply chain security by simplifying cryptographic signing and verifying of software artifacts, such as container images, binaries and documents. Trusted Artifact Signer provides a production ready deployment of the Sigstore project within an enterprise. Enterprises adopting it can meet signing-related criteria for achieving Supply Chain Levels for Software Artifacts (SLSA) compliance and have greater confidence in the security and trustworthiness of their software supply chains.

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use an OpenShift Cluster or a Kubernetes cluster to install this operator.

### Running on the cluster
1. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/operator:tag
```

2. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/operator:tag
```

3. Once the controller has been deployed modify the sample deployment located at `config/samples/rhtas_v1alpha1_securesign.yaml` then deploy.
NOTE: You will need an OIDC provider. This can be Amazon or Keycloak for example.

```sh
kubectl apply -f config/samples/rhtas_v1alpha1_securesign.yaml
```

4. The components have now been deployed to the Kubernetes cluster and available to be used with Cosign and Tekton chains to sign artifacts.
NOTE: Please define the values of YOUR_KEYCLOAK and YOUR_REALM for the OIDC provider, USER, PASSWORD, IMAGE, and OCP_APPS_URL. Finally specify a container image that you have authorization to push images to.

```sh
export OIDC_ISSUER_URL=https://$YOUR_KEYCLOAK/auth/realms/$YOUR_REALM
TOKEN=$(curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "username=$USER" -d "password=$PASSWORD" -d "grant_type=password" -d "scope=openid" -d "client_id=$YOUR_REALM" $OIDC_ISSUER_URL/protocol/openid-connect/token |  sed -E 's/.*"access_token":"([^"]*).*/\1/')
cosign initialize --mirror=https://tuf.$OCP_APPS_URL/ --root=https://tuf.$OCP_APPS_URL/root.json
cosign sign -y --fulcio-url=https://fulcio.$OCP_APPS_URL/ --oidc-issuer=$OIDC_ISSUER_URL --identity-token=$TOKEN $IMAGE
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```

### Local Development
#### Install the CRDs into the cluster:
```
make install
````

Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):
```
make run
```
NOTE: You can also run this in one step by running: make install run

## EKS deployment
It is possible to run RHTAS on EKS. If image building and signing all occurs within the cluster Ingress and Certifcates are not required. However, this will make it difficult to verify the image signatures from outside the cluster. It is highly suggested to deploy with Ingress and Certificates in place.

A script located at `ci/eks.sh` is provided that will prompt for a few starting values and then deploy an EKS cluster that will be suitable for an EKS deployment.

### Deploy RHTAS on EKS
Once a suitable EKS environment has been deployed either an existing install or one using the `ci/eks.sh` script perform the following using addresses that relate to your environment.

NOTE: If you did not use the `ci/eks.sh` script please install the operator by performing the following.
```
IMG=quay.io/redhat-user-workloads/rhtas-tenant/operator/rhtas-operator@sha256:584b4e47603e17c45d4b2a3bf7c71e9a4d249e2f50619fa1e6a57b6742d2e2ad make deploy
```

```
kubectl create ns securesign
```

Deploy RHTAS using values that relate to your certificate, ODIC provider, and DNS. Shown below is a sample deployment file.
```
apiVersion: rhtas.redhat.com/v1alpha1
kind: Securesign
metadata:
  labels:
    app.kubernetes.io/name: securesign-sample
    app.kubernetes.io/instance: securesign-sample
    app.kubernetes.io/part-of: trusted-artifact-signer
  name: securesign-sample
  namespace: rhtas-operator
spec:
  rekor:
    externalAccess:
      enabled: true
      host: rekor.example.com
    monitoring:
      enabled: false
  trillian:
    database:
      create: true
  fulcio:
    externalAccess:
      enabled: true
      host: fulcio.example.com
    config:
      OIDCIssuers:
        - ClientID: "trusted-artifact-signer"
          IssuerURL: "https://your-oidc-issuer-url"
          Issuer: "https://your-oidc-issuer-url"
          Type: "email"
    certificate:
      organizationName: Red Hat
      organizationEmail: jdoe@redhat.com
      commonName: fulcio.example.com
    monitoring:
      enabled: false
  tuf:
    externalAccess:
      enabled: true
      host: tuf.example.com
  ctlog:
```

Apply the configuration file above once the values for your environment are defined.
```
kubectl apply -n securesign -f rhtas-deployment.yaml
```

### Verifying signatures
To verify RHTAS is working as expected we will initialize to TUF.
```
export TUF_URL=https://tuf.example.com
cosign initialize --mirror=$TUF_URL --root=$TUF_URL/root.json
```
The output of the command will appear similar to below.
```
Root status:
 {
	"local": "/home/$USER/.sigstore/root",
	"remote": "https://tuf.example.com",
	"metadata": {
		"root.json": {
			"version": 1,
			"len": 2178,
			"expiration": "14 Sep 24 16:40 UTC",
			"error": ""
		},
....
```

It is now possible to sign an image. The example below will authenticate to `Keycloak` to generate a token.
```
podman pull alpine
podman tag alpine ttl.sh/tas-test:5m
podman push ttl.sh/tas-test:5m
export FULCIO_URL=https://fulcio.example.com
export REKOR_URL=https://rekor.example.com
export OIDC_ISSUER_URL=https://keycloak-keycloak-system.example/auth/realms/trusted-artifact-signer
TOKEN=$(curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "username=jdoe" -d "password=secure" -d "grant_type=password" -d "scope=openid" -d "client_id=trusted-artifact-signer" $OIDC_ISSUER_URL/protocol/openid-connect/token |  sed -E 's/.*"access_token":"([^"]*).*/\1/')
cosign sign -y --fulcio-url=$FULCIO_URL --rekor-url=$REKOR_URL --oidc-issuer=$OIDC_ISSUER_URL --oidc-client-id=trusted-artifact-signer --identity-token=$TOKEN ttl.sh/tas-test:
cosign verify --rekor-url=\$REKOR_URL --certificate-identity-regexp ".*@redhat" --certificate-oidc-issuer-regexp ".*keycloak.*" ttl.sh/tas-test:5m
```

## Clients
RHTAS provides client binaries for cosign, gitsign, rekor-cli, and ec. To access these resources the ingress resource must exist. To do this create the following file `cli-ingress.yaml` using your wildcard domain.

```
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cli-external
  namespace: trusted-artifact-signer
spec:
  ingressClassName: nginx
  rules:
  - host: cli-server.example.com
    http:
      paths:
      - backend:
          service:
            name: cli-server
            port:
              name: cli-server
        path: /clients(/|$)(.*)
        pathType: ImplementationSpecific
```

Apply the file to the cluster.

```
kubectl apply -f cli-ingress.yaml
```
