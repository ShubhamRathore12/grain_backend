#!/bin/bash

# Deployment script for grain_backend
# Usage: bash deploy.sh

echo "🚀 Starting deployment to production server..."
echo "📍 Server: 91.98.235.142"

# Deploy to server
ssh -i ~/.ssh/ssh-key.key root@91.98.235.142 << 'EOF'

echo "✅ Connected to server"
cd /opt/grain_backend

echo "📥 Pulling latest changes from git..."
git pull origin main

echo "🐳 Building Docker image and starting containers..."
docker compose up -d --build --force-recreate

echo "🧹 Cleaning up unused Docker images..."
docker image prune -f

echo "✅ Deployment complete!"
echo "🔍 Checking container status..."
docker ps

EOF

echo "✅ Deployment finished!"
