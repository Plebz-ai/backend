#!/bin/bash
set -e

echo "Building and testing backend Docker image..."

# Build the Docker image
docker build -t ai-character-demo-backend:local .

echo "Build completed successfully."
echo "Testing the container..."

# Clean up any existing test container
docker rm -f ai-character-demo-test 2>/dev/null || true

# Run a test container with all necessary environment variables
CONTAINER_ID=$(docker run -d --name ai-character-demo-test -p 8089:8081 \
  -e PORT=8081 \
  -e ENV=development \
  -e DB_HOST=host.docker.internal \
  -e DB_PORT=5432 \
  -e DB_NAME=ai_character_db \
  -e DB_USER=postgres \
  -e DB_PASSWORD=postgres \
  -e USE_MOCK_AI=true \
  -e JWT_SECRET=test-jwt-secret-for-development-only \
  -e OPENAI_API_KEY=sk-test \
  -e ELEVENLABS_API_KEY=test-key \
  -e AI_LAYER_URL=http://localhost:8000 \
  ai-character-demo-backend:local)

echo "Container started with ID: $CONTAINER_ID"
echo "Waiting for the application to start..."

# Wait for the container to be ready
sleep 10

# Test the health endpoint
if curl -s -f http://localhost:8089/api/health > /dev/null; then
  echo "✅ Health check succeeded!"
  curl -s http://localhost:8089/api/health
else
  echo "❌ Health check failed!"
  echo "Container logs:"
  docker logs $CONTAINER_ID
  docker stop $CONTAINER_ID
  docker rm $CONTAINER_ID
  exit 1
fi

# Clean up
docker stop $CONTAINER_ID
docker rm $CONTAINER_ID

echo "✅ Docker image test passed!"
echo "You can now use the docker-compose.yml file to run the full stack locally:"
echo "docker-compose up -d"
echo "Or deploy to production using the deploy.sh script" 