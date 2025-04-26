#!/bin/bash
set -e

# Configuration
IMAGE_NAME="ai-character-demo-backend"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
REGISTRY="your-docker-registry.io" # Replace with your registry, e.g., ghcr.io/username or docker.io/username

# Build the Docker image
echo "Building Docker image: $IMAGE_NAME:$VERSION"
docker build -t $IMAGE_NAME:$VERSION .
docker tag $IMAGE_NAME:$VERSION $IMAGE_NAME:latest

# Tag with registry
REGISTRY_TAG="$REGISTRY/$IMAGE_NAME:$VERSION"
LATEST_TAG="$REGISTRY/$IMAGE_NAME:latest"
docker tag $IMAGE_NAME:$VERSION $REGISTRY_TAG
docker tag $IMAGE_NAME:$VERSION $LATEST_TAG

# Login to registry (uncomment and use if needed)
# echo "Logging into Docker registry"
# docker login $REGISTRY

# Push the image
echo "Pushing Docker image to registry"
docker push $REGISTRY_TAG
docker push $LATEST_TAG

echo "Deployment preparation complete!"
echo "Image: $REGISTRY_TAG"
echo ""
echo "Next steps:"
echo "1. Update your Kubernetes deployment or docker-compose.yml with the new image"
echo "2. Apply the new configuration to your environment"
echo "3. Verify the deployment with 'kubectl get pods' or 'docker ps'" 