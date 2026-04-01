#!/bin/bash

# Configuration
IMAGE_NAME="regis.pointit.co.th/ata/gateway-api"
DEPLOY_FILE="deploy/aisom/prod/deploy.yaml"

# Ensure we are in the right directory
if [ ! -f "Dockerfile" ]; then
    echo "❌ Error: Dockerfile not found. Please run this script from the gateway-api directory."
    exit 1
fi

# Get the short Git commit hash
COMMIT_HASH=$(git rev-parse --short HEAD)

# Check if git command was successful
if [ $? -ne 0 ]; then
    echo "❌ Error: Failed to get Git commit hash. Is this a git repository?"
    exit 1
fi

# Optionally allow passing a custom tag or suffix as the first argument
if [ -n "$1" ]; then
    if [[ "$1" == -* ]]; then
        TAG="${COMMIT_HASH}${1}"
    else
        TAG="$1"
    fi
else
    TAG="${COMMIT_HASH}"
fi

FULL_IMAGE="${IMAGE_NAME}:${TAG}"

echo "========================================"
echo "🚀 Starting build and deploy process for gateway-api"
echo "📦 Image Tag:    $TAG"
echo "📝 Deploy File:  $DEPLOY_FILE"
echo "========================================"

# 1. Build the image
echo "⏳ [1/4] Building image using podman..."
podman build -f Dockerfile -t "$FULL_IMAGE" .

if [ $? -ne 0 ]; then
    echo "❌ Build failed. Exiting."
    exit 1
fi

# 2. Push the image
echo "📤 [2/4] Pushing image to registry..."
podman push "$FULL_IMAGE"

if [ $? -ne 0 ]; then
    echo "❌ Push failed. Exiting."
    exit 1
fi

# 3. Update the deployment file
echo "📝 [3/4] Updating deployment manifest..."
if [ -f "$DEPLOY_FILE" ]; then
    # Use sed to replace the image tag in the deployment file
    sed -i "s|image: ${IMAGE_NAME}:.*|image: ${IMAGE_NAME}:${TAG}|g" "$DEPLOY_FILE"
    echo "✅ Successfully updated $DEPLOY_FILE to use image: $FULL_IMAGE"
else
    echo "⚠️ Warning: Deploy file $DEPLOY_FILE not found. Please verify the path."
    exit 1
fi

# 4. Apply the deployment
echo "🚀 [4/4] Applying deployment to Kubernetes cluster..."
# Note: We need to make sure the secret is updated if .env.dev has changed,
# but for standard deployment updates, applying the yaml is sufficient.
kubectl apply -f "$DEPLOY_FILE"

if [ $? -ne 0 ]; then
    echo "❌ Deployment failed. Exiting."
    exit 1
fi

echo "========================================"
echo "🎉 Build, Push, and Deploy completed successfully!"
echo "========================================"
