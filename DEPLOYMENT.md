# Backend Deployment Guide

This guide explains how to deploy the AI Character Demo backend service to production.

## Prerequisites

- Docker
- Docker Compose (for local testing)
- Access to a Docker registry (for production deployment)

## Local Testing with Docker Compose

1. Build and start the containers:
   ```
   docker-compose up -d
   ```

2. Verify the application is running:
   ```
   curl http://localhost:8081/health
   ```

3. To stop the containers:
   ```
   docker-compose down
   ```

## Production Deployment

### Environment Configuration

1. Copy the production environment template:
   ```
   cp .env.production .env
   ```

2. Edit `.env` and replace placeholder values with your actual secrets.

### Building and Pushing the Docker Image

1. Configure your Docker registry in `deploy.sh` (replace "your-docker-registry.io" with your actual registry URL).

2. Run the deployment script:
   ```
   ./deploy.sh
   ```

### Kubernetes Deployment (Recommended for Production)

1. Create Kubernetes secrets for sensitive information:
   ```
   kubectl create secret generic ai-character-demo-secrets \
     --from-literal=DB_PASSWORD=your-db-password \
     --from-literal=JWT_SECRET=your-jwt-secret \
     --from-literal=OPENAI_API_KEY=your-openai-key \
     --from-literal=ELEVENLABS_API_KEY=your-elevenlabs-key
   ```

2. Create a Kubernetes deployment file (`k8s-deployment.yaml`):
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: ai-character-demo-backend
     labels:
       app: ai-character-demo-backend
   spec:
     replicas: 2
     selector:
       matchLabels:
         app: ai-character-demo-backend
     template:
       metadata:
         labels:
           app: ai-character-demo-backend
       spec:
         containers:
         - name: backend
           image: your-docker-registry.io/ai-character-demo-backend:latest
           ports:
           - containerPort: 8081
           envFrom:
           - configMapRef:
               name: ai-character-demo-config
           - secretRef:
               name: ai-character-demo-secrets
           resources:
             limits:
               memory: "512Mi"
               cpu: "500m"
             requests:
               memory: "256Mi"
               cpu: "250m"
           livenessProbe:
             httpGet:
               path: /health
               port: 8081
             initialDelaySeconds: 30
             periodSeconds: 30
           readinessProbe:
             httpGet:
               path: /health
               port: 8081
             initialDelaySeconds: 5
             periodSeconds: 10
   ```

3. Apply the deployment:
   ```
   kubectl apply -f k8s-deployment.yaml
   ```

### Server Deployment

If you prefer deploying directly on a server:

1. Set up your server with Docker installed.

2. Copy the Docker Compose file and .env.production to your server.

3. Configure the environment variables:
   ```
   cp .env.production .env
   # Edit .env with actual production values
   ```

4. Run the application:
   ```
   docker-compose up -d
   ```

## Database Migrations

The application automatically runs database migrations on startup. For large production deployments, consider running migrations separately before deploying the new version:

```
docker run --rm \
  --env-file .env \
  your-docker-registry.io/ai-character-demo-backend:latest \
  /app/main migrate
```

## Monitoring

The application exposes a `/health` endpoint for health checking and basic monitoring. For comprehensive monitoring, consider setting up:

- Prometheus for metrics
- ELK or Grafana Loki for logs
- Jaeger or Zipkin for distributed tracing 