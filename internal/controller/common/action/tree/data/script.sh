set -e

# Define API server variables
APISERVER="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)

# Use the environment variable CONFIGMAP_NAME or default to "tree-config"
CONFIGMAP_NAME=${CONFIGMAP_NAME:-tree-config}

TREE_ID=$(/createtree --rpc_deadline=240s --admin_server="${ADMIN_SERVER}" --display_name="${DISPLAY_NAME}" ${EXTRA_ARGS})
if [ $? -ne 0 ]; then
    echo "Failed to create tree" >&2
    exit 1
fi
echo "Created tree with id: $TREE_ID"

echo "Updating ${CONFIGMAP_NAME} ConfigMap..."
# Update the ConfigMap with a strategic merge patch
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --cacert ${CACERT} \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/strategic-merge-patch+json" \
  -X PATCH ${APISERVER}/api/v1/namespaces/${NAMESPACE}/configmaps/${CONFIGMAP_NAME} \
  -d "{\"data\": {\"tree_id\": \"${TREE_ID}\"}}")

if [ "$HTTP_CODE" -ne "200" ]; then
  echo "Failed to PATCH ${CONFIGMAP_NAME} ConfigMap, HTTP_CODE: ${HTTP_CODE}" >&2
  exit 1
fi

echo "Success"
